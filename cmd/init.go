package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/claude"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/schedule"
	"github.com/stevejkang/tokfresh-cli/internal/subscribe"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard for a new TokFresh instance",
	Long:  "Walks you through connecting a Claude account, configuring a schedule, and deploying a Cloudflare Worker.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ui.EnterAltScreen()
	defer ui.ExitAltScreen()

	detectedTZ := schedule.DetectTimezone()

	authURL, verifier, err := claude.GenerateAuthURL()
	if err != nil {
		return fmt.Errorf("failed to generate auth URL: %w", err)
	}

	result, err := ui.RunSetupWizard(detectedTZ, authURL)
	if err != nil {
		return fmt.Errorf("setup wizard cancelled: %w", err)
	}

	// Resolve worker name
	workerName := result.WorkerName
	if workerName == "" {
		workerName = config.GenerateWorkerName("")
	} else {
		workerName = config.GenerateWorkerName(workerName)
	}

	var tokenResp *claude.TokenResponse
	err = ui.RunWithSpinnerFullscreen("Exchanging authorization code...", func() error {
		var exchangeErr error
		tokenResp, exchangeErr = claude.ExchangeCode(result.AuthCode, verifier)
		return exchangeErr
	})
	if err != nil {
		return fmt.Errorf("OAuth exchange failed: %w\n  Hint: try running `tokfresh init` again", err)
	}

	auth, err := resolveCloudflareAuth(result.CloudflareAuthMode)
	if err != nil {
		return fmt.Errorf("Cloudflare auth failed: %w", err)
	}

	var cfResult *cloudflare.VerifyResult
	err = ui.RunWithSpinnerFullscreen("Verifying Cloudflare account...", func() error {
		var verifyErr error
		cfResult, verifyErr = cloudflare.VerifyToken(auth.Token)
		return verifyErr
	})
	if err != nil {
		return fmt.Errorf("Cloudflare token verification failed: %w", err)
	}
	auth.AccountID = cfResult.AccountID

	cfLabel := cfResult.AccountName
	if cfResult.Email != "" {
		cfLabel += " (" + cfResult.Email + ")"
	}

	confirmCF := true
	ui.ClearAndBrand()
	cfForm := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Deploy to this Cloudflare account?").
			Description(cfLabel).
			Affirmative("Yes").
			Negative("No").
			Value(&confirmCF),
	))
	cfForm.WithTheme(ui.TokFreshTheme())
	if err := cfForm.Run(); err != nil || !confirmCF {
		return fmt.Errorf("cancelled")
	}

	slots := schedule.Calculate(result.StartTime)
	cronExpr := schedule.ToCron(slots, result.Timezone)
	log.Debug("schedule calculated", "slots", slots, "cron", cronExpr)

	notifConfig := result.BuildNotificationConfig()

	workerCode := cloudflare.GenerateWorkerCode()
	err = ui.RunWithSpinnerFullscreen("Deploying worker...", func() error {
		return cloudflare.Deploy(cloudflare.DeployParams{
			Token:              auth.Token,
			AccountID:          auth.AccountID,
			WorkerName:         workerName,
			WorkerCode:         workerCode,
			RefreshToken:       tokenResp.RefreshToken,
			CronExpression:     cronExpr,
			Timezone:           result.Timezone,
			NotificationConfig: notifConfig,
			OnProgress: func(step string) {
				log.Info(step)
			},
		})
	})
	if err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	inst := config.Instance{
		Name:                workerName,
		CloudflareAccountID: auth.AccountID,
		Schedule:            result.StartTime,
		Timezone:            result.Timezone,
		CronExpression:      cronExpr,
	}
	if result.NotificationType != "none" {
		inst.NotificationType = result.NotificationType
	}

	if err := config.AddInstance(inst); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	_, nextLabel := schedule.GetNextTrigger(slots, result.Timezone)

	successNote := fmt.Sprintf("✓ Worker %q is live.\nNext trigger: %s", workerName, nextLabel)

	var email string
	ui.ClearAndBrand()
	doneForm := huh.NewForm(huh.NewGroup(
		huh.NewNote().
			Title("Setup Complete").
			Description(successNote),
		huh.NewInput().
			Title("Email for breaking change notifications (optional)").
			Description("We'll only email when your workers need updating. Press Enter to skip.").
			Placeholder("user@example.com").
			Value(&email),
	))
	doneForm.WithTheme(ui.TokFreshTheme())
	if err := doneForm.Run(); err != nil {
		log.Debug("email prompt cancelled", "error", err)
	}

	if email != "" {
		if subErr := subscribe.Subscribe(email); subErr != nil {
			log.Warn("subscription failed", "error", subErr)
		}
	}

	ui.ExitAltScreen()

	fmt.Println()
	fmt.Println(ui.SuccessStyle.Render("  ✓ Setup complete!"))
	fmt.Println()
	fmt.Printf("  Worker:       %s\n", workerName)
	fmt.Printf("  Schedule:     %s (%s)\n", result.StartTime, result.Timezone)
	fmt.Printf("  Next trigger: %s\n", nextLabel)
	fmt.Printf("  Console:      %s\n", cloudflare.ConsoleURL(auth.AccountID, workerName))
	fmt.Println()

	return nil
}

func resolveCloudflareAuth(mode string) (*cloudflare.AuthResult, error) {
	switch mode {
	case "env":
		result := cloudflare.ResolveFromEnv()
		if result == nil {
			return nil, fmt.Errorf("CLOUDFLARE_API_TOKEN environment variable is not set")
		}
		return result, nil

	case "wrangler":
		token, err := cloudflare.GetWranglerToken()
		if err != nil {
			return nil, err
		}
		return &cloudflare.AuthResult{Token: token, Source: "wrangler"}, nil

	case "wrangler-login":
		ui.ExitAltScreen()
		fmt.Println(ui.MutedStyle.Render("  Opening browser for Cloudflare login..."))
		if err := cloudflare.RunWranglerLogin(); err != nil {
			ui.EnterAltScreen()
			return nil, fmt.Errorf("wrangler login failed: %w", err)
		}
		ui.EnterAltScreen()
		token, err := cloudflare.GetWranglerToken()
		if err != nil {
			return nil, fmt.Errorf("wrangler login succeeded but token retrieval failed: %w", err)
		}
		return &cloudflare.AuthResult{Token: token, Source: "wrangler"}, nil

	case "wrangler-install":
		ui.ExitAltScreen()
		fmt.Println(ui.MutedStyle.Render("  Installing wrangler..."))
		if err := cloudflare.InstallWrangler(); err != nil {
			ui.EnterAltScreen()
			return nil, fmt.Errorf("wrangler install failed: %w\n  Fallback: enter API token manually", err)
		}
		fmt.Println(ui.SuccessStyle.Render("  ✓ Wrangler installed"))
		fmt.Println(ui.MutedStyle.Render("  Opening browser for Cloudflare login..."))
		if err := cloudflare.RunWranglerLogin(); err != nil {
			ui.EnterAltScreen()
			return nil, fmt.Errorf("wrangler login failed: %w", err)
		}
		ui.EnterAltScreen()
		token, err := cloudflare.GetWranglerToken()
		if err != nil {
			return nil, err
		}
		return &cloudflare.AuthResult{Token: token, Source: "wrangler"}, nil

	case "manual":
		var token string
		ui.ClearAndBrand()
		tokenForm := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Cloudflare API Token").
				Description("Create at: https://dash.cloudflare.com/profile/api-tokens").
				EchoMode(huh.EchoModePassword).
				Value(&token),
		))
		tokenForm.WithTheme(ui.TokFreshTheme())
		if err := tokenForm.Run(); err != nil {
			return nil, fmt.Errorf("token input cancelled: %w", err)
		}
		if token == "" {
			return nil, fmt.Errorf("API token is required")
		}
		return &cloudflare.AuthResult{Token: token, Source: "manual"}, nil
	}

	return nil, fmt.Errorf("unknown auth mode: %s", mode)
}
