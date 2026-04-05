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

	auth, authErr := ensureAuthForInstance(inst)

	cfDeleteOK := false
	if authErr == nil {
		deleteErr := ui.RunWithSpinnerFullscreen("Deleting worker from Cloudflare...", func() error {
			return cloudflare.DeleteWorker(auth.AccountID, auth.Token, name)
		})
		if deleteErr != nil {
			log.Warn("failed to delete worker from Cloudflare", "error", deleteErr)
		} else {
			cfDeleteOK = true
		}
	}

	if !cfDeleteOK {
		ui.ExitAltScreen()

		var removeLocal bool
		localForm := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Remove from local config only?").
				Description("The worker could not be deleted from Cloudflare. Remove it from local config anyway?").
				Affirmative("Yes").
				Negative("No").
				Value(&removeLocal),
		))
		localForm.WithTheme(ui.TokFreshTheme())
		if err := localForm.Run(); err != nil || !removeLocal {
			fmt.Println()
			fmt.Println(ui.MutedStyle.Render("  Cancelled. No changes made."))
			fmt.Println()
			return nil
		}
	}

	if removeErr := config.RemoveInstance(name); removeErr != nil {
		return fmt.Errorf("failed to remove from config: %w", removeErr)
	}

	ui.ExitAltScreen()

	fmt.Println()
	if cfDeleteOK {
		fmt.Println(ui.SuccessStyle.Render("  ✓ Removed!"))
	} else {
		fmt.Println(ui.SuccessStyle.Render("  ✓ Removed from local config"))
		fmt.Println(ui.MutedStyle.Render("  ⚠ Worker may still exist on Cloudflare."))
	}
	fmt.Printf("  Worker %q deleted.\n", name)
	fmt.Println()

	return nil
}
