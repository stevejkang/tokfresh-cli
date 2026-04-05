package cmd

import (
	"errors"
	"fmt"

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

	type skippedInstance struct {
		Name        string
		AccountName string
	}

	var accessible []config.Instance
	var inaccessible []skippedInstance

	for _, inst := range instances {
		accountID := inst.CloudflareAccountID
		if accountID == "" {
			accessible = append(accessible, inst)
			continue
		}
		if checkErr := cloudflare.EnsureAccountAccess(auth.Token, accountID); checkErr != nil {
			if errors.Is(checkErr, cloudflare.ErrUnauthorized) {
				label := inst.CloudflareAccountName
				if label == "" {
					label = accountID
				}
				inaccessible = append(inaccessible, skippedInstance{Name: inst.Name, AccountName: label})
			} else {
				log.Warn("account access check failed", "worker", inst.Name, "error", checkErr)
				accessible = append(accessible, inst)
			}
		} else {
			accessible = append(accessible, inst)
		}
	}

	workerCode := cloudflare.GenerateWorkerCode()
	upgraded := 0
	failed := 0

	if len(accessible) > 0 {
		fmt.Println()
		fmt.Printf("  %s\n", ui.BoldStyle.Render(fmt.Sprintf("Upgrading workers to TokFresh %s template...", Version)))

		for _, inst := range accessible {
			fmt.Printf("\n  %s\n", inst.Name)

			accountID := inst.CloudflareAccountID
			if accountID == "" {
				accountID = auth.AccountID
			}

			kvTitle := fmt.Sprintf("tokfresh-tokens-%s", inst.Name)
			nsID, findErr := cloudflare.FindKV(accountID, auth.Token, kvTitle)
			if findErr != nil {
				fmt.Printf("    %s KV namespace not found: %v\n", ui.ErrorStyle.Render("✗"), findErr)
				failed++
				continue
			}
			log.Debug("found KV namespace", "title", kvTitle, "id", nsID)

			if uploadErr := cloudflare.UploadWorker(accountID, auth.Token, inst.Name, workerCode, nsID); uploadErr != nil {
				fmt.Printf("    %s Upload failed: %v\n", ui.ErrorStyle.Render("✗"), uploadErr)
				failed++
				continue
			}
			fmt.Printf("    %s Worker code updated\n", ui.SuccessStyle.Render("✓"))
			fmt.Printf("    %s Cron schedule unchanged\n", ui.SuccessStyle.Render("✓"))
			upgraded++
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
		}
		fmt.Println()
	}

	return nil
}
