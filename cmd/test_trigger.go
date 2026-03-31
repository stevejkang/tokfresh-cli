package cmd

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
)

var testDryRun bool

var testCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Manually trigger a worker's scheduled handler",
	Long:  "Deploys a test-enabled worker with an HTTP route, triggers it, then restores the production worker code. Use --dry-run to preview without deploying.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTest,
}

func init() {
	testCmd.Flags().BoolVar(&testDryRun, "dry-run", false, "Show what would happen without deploying")
	rootCmd.AddCommand(testCmd)
}

func runTest(cmd *cobra.Command, args []string) error {
	name := args[0]

	inst := config.GetInstance(name)
	if inst == nil {
		return instanceNotFoundError(name)
	}

	if testDryRun {
		fmt.Println()
		fmt.Println(ui.BoldStyle.Render("  [DRY RUN] Would perform the following:"))
		fmt.Printf("  1. Deploy test-enabled worker for %q\n", name)
		fmt.Printf("  2. Trigger GET https://%s.<subdomain>.workers.dev/__test\n", name)
		fmt.Println("  3. Restore production worker code")
		fmt.Println()
		return nil
	}

	auth, err := resolveCloudflareAuthAuto()
	if err != nil {
		return err
	}

	accountID := auth.AccountID
	if accountID == "" {
		accountID = inst.CloudflareAccountID
	}

	// Find existing KV namespace
	kvTitle := fmt.Sprintf("tokfresh-tokens-%s", name)
	nsID, err := cloudflare.FindKV(accountID, auth.Token, kvTitle)
	if err != nil {
		return fmt.Errorf("KV namespace not found: %w", err)
	}

	testWorkerCode := cloudflare.GenerateTestWorkerCode()

	// Step 1: Deploy test-enabled worker
	fmt.Println()
	err = ui.RunWithSpinner("  Deploying test-enabled worker...", func() error {
		return cloudflare.UploadWorker(accountID, auth.Token, name, testWorkerCode, nsID)
	})
	if err != nil {
		return fmt.Errorf("failed to deploy test worker: %w", err)
	}
	fmt.Println(ui.SuccessStyle.Render("  ✓ Test worker deployed"))

	// Brief delay for propagation
	time.Sleep(2 * time.Second)

	// Step 2: Trigger the test endpoint
	triggerURL := fmt.Sprintf("https://%s.workers.dev/__test", name)
	fmt.Println(ui.MutedStyle.Render(fmt.Sprintf("  Triggering %s...", triggerURL)))

	resp, triggerErr := http.Get(triggerURL)
	if triggerErr != nil {
		log.Warn("trigger request failed", "error", triggerErr)
		fmt.Println(ui.ErrorStyle.Render(fmt.Sprintf("  ✗ Trigger failed: %v", triggerErr)))
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Println(ui.SuccessStyle.Render("  ✓ Worker executed successfully"))
		} else {
			fmt.Println(ui.ErrorStyle.Render(fmt.Sprintf("  ✗ Worker returned %d: %s", resp.StatusCode, string(body))))
		}
	}

	// Step 3: Restore production worker
	productionCode := cloudflare.GenerateWorkerCode()
	err = ui.RunWithSpinner("  Restoring production worker...", func() error {
		return cloudflare.UploadWorker(accountID, auth.Token, name, productionCode, nsID)
	})
	if err != nil {
		fmt.Println(ui.ErrorStyle.Render(fmt.Sprintf("  ⚠ Failed to restore production worker: %v", err)))
		fmt.Println(ui.ErrorStyle.Render("  ⚠ Your worker currently has the test route exposed."))
		fmt.Println(ui.MutedStyle.Render("  Run `tokfresh upgrade` to restore production code."))
		return err
	}
	fmt.Println(ui.SuccessStyle.Render("  ✓ Production worker restored"))

	fmt.Println()
	return nil
}
