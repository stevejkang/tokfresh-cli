package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
)

var dryRun bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Re-deploy all workers with the latest worker template",
	Long:  "Updates the worker code on all managed instances. Use after updating the CLI to push new API headers, model names, or bug fixes to your deployed workers.",
	RunE:  runUpgrade,
}

func init() {
	upgradeCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would change without deploying")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	instances := config.ListInstances()
	if len(instances) == 0 {
		fmt.Println(ui.MutedStyle.Render("  No instances to upgrade. Run `tokfresh init` first."))
		return nil
	}

	if dryRun {
		fmt.Println(ui.BoldStyle.Render("  [DRY RUN] Would upgrade the following workers:"))
		fmt.Println()
		for _, inst := range instances {
			fmt.Printf("  • %s\n", inst.Name)
		}
		fmt.Printf("\n  %d worker(s) would be upgraded.\n", len(instances))
		return nil
	}

	auth, err := resolveCloudflareAuthAuto()
	if err != nil {
		return err
	}

	var verifyResult *cloudflare.VerifyResult
	verifyResult, err = cloudflare.VerifyToken(auth.Token)
	if err != nil {
		log.Debug("token verify for email backfill failed", "error", err)
	}

	type skippedInstance struct {
		Name             string
		AccountID        string
		AccountName      string
		CloudflareEmail  string
		OriginalInstance config.Instance
	}

	type upgradeTarget struct {
		Instance config.Instance
		Token    string
	}

	var accessible []upgradeTarget
	var inaccessible []skippedInstance

	for _, inst := range instances {
		accountID := inst.CloudflareAccountID
		if accountID == "" {
			accessible = append(accessible, upgradeTarget{Instance: inst, Token: auth.Token})
			continue
		}
		if checkErr := cloudflare.EnsureAccountAccess(auth.Token, accountID); checkErr != nil {
			if errors.Is(checkErr, cloudflare.ErrUnauthorized) {
				label := inst.CloudflareAccountName
				if label == "" {
					label = accountID
				}
				inaccessible = append(inaccessible, skippedInstance{
					Name:             inst.Name,
					AccountID:        accountID,
					AccountName:      label,
					CloudflareEmail:  inst.CloudflareEmail,
					OriginalInstance: inst,
				})
			} else {
				log.Warn("account access check failed", "worker", inst.Name, "error", checkErr)
				accessible = append(accessible, upgradeTarget{Instance: inst, Token: auth.Token})
			}
		} else {
			accessible = append(accessible, upgradeTarget{Instance: inst, Token: auth.Token})
		}
	}

	if len(inaccessible) > 0 && auth.Source != "env" {
		type accountGroup struct {
			AccountID   string
			AccountName string
			Email       string
			Workers     []skippedInstance
		}
		groupOrder := []string{}
		groups := map[string]*accountGroup{}
		for _, s := range inaccessible {
			g, exists := groups[s.AccountID]
			if !exists {
				g = &accountGroup{
					AccountID:   s.AccountID,
					AccountName: s.AccountName,
					Email:       s.CloudflareEmail,
				}
				groups[s.AccountID] = g
				groupOrder = append(groupOrder, s.AccountID)
			}
			g.Workers = append(g.Workers, s)
			if g.Email == "" && s.CloudflareEmail != "" {
				g.Email = s.CloudflareEmail
			}
		}

		var stillInaccessible []skippedInstance

		for _, acctID := range groupOrder {
			g := groups[acctID]

			acctLabel := g.AccountName
			if g.Email != "" {
				acctLabel += " (" + g.Email + ")"
			}

			fmt.Println()
			fmt.Printf("  %s\n", ui.ErrorStyle.Render("⚠ Cannot access account: "+acctLabel))
			fmt.Println("  Workers needing this account:")
			for _, w := range g.Workers {
				fmt.Printf("    • %s\n", w.Name)
			}

			confirmSwitch := false
			switchForm := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Switch to account '%s' now?", acctLabel)).
					Affirmative("Yes").
					Negative("Skip").
					Value(&confirmSwitch),
			))
			switchForm.WithTheme(ui.TokFreshTheme())
			if formErr := switchForm.Run(); formErr != nil {
				log.Debug("re-auth prompt cancelled", "account", acctLabel, "error", formErr)
				stillInaccessible = append(stillInaccessible, g.Workers...)
				continue
			}

			if !confirmSwitch {
				stillInaccessible = append(stillInaccessible, g.Workers...)
				continue
			}

			ui.ExitAltScreen()
			fmt.Println(ui.MutedStyle.Render("  Switching Cloudflare account..."))
			fmt.Println()
			if logoutErr := cloudflare.RunWranglerLogout(); logoutErr != nil {
				log.Debug("wrangler logout failed (may not have been logged in)", "error", logoutErr)
			}
			fmt.Println()
			fmt.Println(ui.MutedStyle.Render("  Opening browser for Cloudflare login..."))
			if loginErr := cloudflare.RunWranglerLogin(); loginErr != nil {
				ui.EnterAltScreen()
				log.Warn("wrangler login failed", "error", loginErr)
				stillInaccessible = append(stillInaccessible, g.Workers...)
				continue
			}
			ui.EnterAltScreen()

			token, tokenErr := cloudflare.GetWranglerToken()
			if tokenErr != nil {
				log.Warn("wrangler token retrieval failed after re-auth", "error", tokenErr)
				stillInaccessible = append(stillInaccessible, g.Workers...)
				continue
			}
			groupToken := token

			if accessErr := cloudflare.EnsureAccountAccess(groupToken, g.AccountID); accessErr != nil {
				log.Warn("account still inaccessible after re-auth", "account", g.AccountName, "error", accessErr)
				stillInaccessible = append(stillInaccessible, g.Workers...)
				continue
			}

			for _, w := range g.Workers {
				accessible = append(accessible, upgradeTarget{Instance: w.OriginalInstance, Token: groupToken})
			}
		}

		inaccessible = stillInaccessible
	}

	var emailsBackfilled []string

	workerCode := cloudflare.GenerateWorkerCode()
	upgraded := 0
	failed := 0

	if len(accessible) > 0 {
		fmt.Println()
		fmt.Printf("  %s\n", ui.BoldStyle.Render(fmt.Sprintf("Upgrading workers to TokFresh %s template...", Version)))

		for _, target := range accessible {
			inst := target.Instance
			fmt.Printf("\n  %s\n", inst.Name)

			accountID := inst.CloudflareAccountID
			if accountID == "" {
				accountID = auth.AccountID
			}

			if verifyResult != nil && inst.CloudflareEmail == "" && inst.CloudflareAccountID == verifyResult.AccountID {
				emailsBackfilled = append(emailsBackfilled, inst.Name)
			}

			kvTitle := fmt.Sprintf("tokfresh-tokens-%s", inst.Name)
			nsID, findErr := cloudflare.FindKV(accountID, target.Token, kvTitle)
			if findErr != nil {
				fmt.Printf("    %s KV namespace not found: %v\n", ui.ErrorStyle.Render("✗"), findErr)
				failed++
				continue
			}
			log.Debug("found KV namespace", "title", kvTitle, "id", nsID)

			if uploadErr := cloudflare.UploadWorker(accountID, target.Token, inst.Name, workerCode, nsID); uploadErr != nil {
				fmt.Printf("    %s Upload failed: %v\n", ui.ErrorStyle.Render("✗"), uploadErr)
				failed++
				continue
			}
			fmt.Printf("    %s Worker code updated\n", ui.SuccessStyle.Render("✓"))
			fmt.Printf("    %s Cron schedule unchanged\n", ui.SuccessStyle.Render("✓"))
			upgraded++
		}
	}

	if len(emailsBackfilled) > 0 && verifyResult != nil {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			log.Warn("failed to load config for email backfill", "error", loadErr)
		} else {
			updated := false
			for i := range cfg.Instances {
				for _, name := range emailsBackfilled {
					if cfg.Instances[i].Name == name && cfg.Instances[i].CloudflareEmail == "" {
						cfg.Instances[i].CloudflareEmail = verifyResult.Email
						updated = true
					}
				}
			}
			if updated {
				if saveErr := config.Save(cfg); saveErr != nil {
					log.Warn("failed to save email backfill", "error", saveErr)
				} else {
					log.Debug("backfilled email for instances", "count", len(emailsBackfilled))
				}
			}
		}
	}

	fmt.Println()
	if upgraded > 0 || failed > 0 {
		if failed > 0 {
			fmt.Printf("  Done! %d worker(s) upgraded, %d failed.\n", upgraded, failed)
		} else {
			fmt.Printf("  %s %d worker(s) upgraded.\n", ui.SuccessStyle.Render("Done!"), upgraded)
		}
	}

	if len(inaccessible) > 0 {
		fmt.Println()
		fmt.Printf("  %s\n", ui.ErrorStyle.Render("⚠ The following workers could not be upgraded (different account):"))
		for _, s := range inaccessible {
			fmt.Printf("    • %s  (%s)\n", s.Name, s.AccountName)
		}
		fmt.Println()

		switch auth.Source {
		case "wrangler":
			fmt.Println(ui.MutedStyle.Render("  Switch to that account and run again:"))
			fmt.Println(ui.MutedStyle.Render("    wrangler login && tokfresh upgrade"))
		case "env":
			fmt.Println(ui.MutedStyle.Render("  Set CLOUDFLARE_API_TOKEN to a token for that account and run `tokfresh upgrade` again."))
		default:
			fmt.Println(ui.MutedStyle.Render("  Provide a token with access to that account."))
		}
		fmt.Println()
	}

	return nil
}
