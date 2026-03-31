package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a TokFresh worker from Cloudflare and local config",
	Long:  "Removes the worker from your Cloudflare account and deletes the instance from local configuration.",
	Args:  cobra.ExactArgs(1),
	RunE:  runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	inst := config.GetInstance(name)
	if inst == nil {
		return instanceNotFoundError(name)
	}

	ui.EnterAltScreen()
	defer ui.ExitAltScreen()

	var confirmed bool
	ui.ClearAndBrand()
	confirmForm := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Delete worker %q?", name)).
			Description("This will remove the worker from Cloudflare and local config.").
			Affirmative("Yes, delete").
			Negative("Cancel").
			Value(&confirmed),
	))
	confirmForm.WithTheme(ui.TokFreshTheme())
	if err := confirmForm.Run(); err != nil {
		return fmt.Errorf("confirmation cancelled: %w", err)
	}
	if !confirmed {
		return nil
	}

	auth, err := resolveCloudflareAuthAuto()
	if err != nil {
		return err
	}

	if auth.AccountID == "" {
		auth.AccountID = inst.CloudflareAccountID
	}

	err = ui.RunWithSpinnerFullscreen("Deleting worker from Cloudflare...", func() error {
		return cloudflare.DeleteWorker(auth.AccountID, auth.Token, name)
	})
	if err != nil {
		log.Warn("failed to delete worker from Cloudflare", "error", err)
	}

	if removeErr := config.RemoveInstance(name); removeErr != nil {
		return fmt.Errorf("failed to remove from config: %w", removeErr)
	}

	ui.ExitAltScreen()

	fmt.Println()
	if err != nil {
		fmt.Println(ui.SuccessStyle.Render("  ✓ Instance removed from local config"))
		fmt.Println(ui.MutedStyle.Render(fmt.Sprintf("  ⚠ Could not delete worker from Cloudflare: %v", err)))
	} else {
		fmt.Println(ui.SuccessStyle.Render("  ✓ Removed!"))
	}
	fmt.Printf("  Worker %q deleted.\n", name)
	fmt.Println()

	return nil
}
