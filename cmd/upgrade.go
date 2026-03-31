package cmd

import (
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

	workerCode := cloudflare.GenerateWorkerCode()
	upgraded := 0
	failed := 0

	fmt.Println()
	fmt.Printf("  %s\n", ui.BoldStyle.Render(fmt.Sprintf("Upgrading workers to TokFresh %s template...", Version)))

	for _, inst := range instances {
		fmt.Printf("\n  %s\n", inst.Name)

		accountID := auth.AccountID
		if accountID == "" {
			accountID = inst.CloudflareAccountID
		}

		// Find existing KV namespace (don't create new one)
		kvTitle := fmt.Sprintf("tokfresh-tokens-%s", inst.Name)
		nsID, findErr := cloudflare.FindKV(accountID, auth.Token, kvTitle)
		if findErr != nil {
			fmt.Printf("    %s KV namespace not found: %v\n", ui.ErrorStyle.Render("✗"), findErr)
			failed++
			continue
		}
		log.Debug("found KV namespace", "title", kvTitle, "id", nsID)

		// Re-upload worker code with existing KV binding
		if uploadErr := cloudflare.UploadWorker(accountID, auth.Token, inst.Name, workerCode, nsID); uploadErr != nil {
			fmt.Printf("    %s Upload failed: %v\n", ui.ErrorStyle.Render("✗"), uploadErr)
			failed++
			continue
		}
		fmt.Printf("    %s Worker code updated\n", ui.SuccessStyle.Render("✓"))
		fmt.Printf("    %s Cron schedule unchanged\n", ui.SuccessStyle.Render("✓"))
		upgraded++
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("  Done! %d worker(s) upgraded, %d failed.\n", upgraded, failed)
	} else {
		fmt.Printf("  %s %d worker(s) upgraded.\n", ui.SuccessStyle.Render("Done!"), upgraded)
	}

	return nil
}
