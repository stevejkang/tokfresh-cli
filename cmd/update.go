package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/schedule"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
)

var updateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update schedule or notifications for a TokFresh instance",
	Long:  "Prompts for new schedule and notification settings, then updates the cron schedule and secrets on Cloudflare. Does not re-auth Claude or re-upload worker code.",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]

	inst := config.GetInstance(name)
	if inst == nil {
		return instanceNotFoundError(name)
	}

	ui.EnterAltScreen()
	defer ui.ExitAltScreen()

	startTime := inst.Schedule
	timezone := inst.Timezone
	notifType := inst.NotificationType
	if notifType == "" {
		notifType = "none"
	}
	var webhookURL string
	var notifyMode string

	ui.ClearAndBrand()
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Start time").
				Description(fmt.Sprintf("Current: %s", inst.Schedule)).
				Options(ui.BuildStartTimeOptions()...).
				Value(&startTime).
				Height(10),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Timezone").
				Description(fmt.Sprintf("Current: %s", inst.Timezone)).
				Options(ui.BuildTimezoneOptions(inst.Timezone)...).
				Value(&timezone).
				Height(12),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Notifications").
				Description("Get notified when the worker triggers").
				Options(
					huh.NewOption("None", "none"),
					huh.NewOption("Slack Webhook", "slack"),
					huh.NewOption("Discord Webhook", "discord"),
				).
				Value(&notifType),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Webhook URL").
				Description("Paste your Slack or Discord webhook URL").
				Placeholder("https://hooks.slack.com/services/...").
				Value(&webhookURL),
			huh.NewSelect[string]().
				Title("When to notify").
				Description("Choose when you receive notifications").
				Options(
					huh.NewOption("All triggers (success + failure)", "all"),
					huh.NewOption("Failures only", "failure"),
				).
				Value(&notifyMode),
		).WithHideFunc(func() bool {
			return notifType == "none"
		}),
	)

	if err := form.WithTheme(ui.TokFreshTheme()).Run(); err != nil {
		return fmt.Errorf("update cancelled: %w", err)
	}

	auth, err := ensureAuthForInstance(inst)
	if err != nil {
		return err
	}

	slots := schedule.Calculate(startTime)
	cronExpr := schedule.ToCron(slots, timezone)

	err = ui.RunWithSpinnerFullscreen("Updating schedule...", func() error {
		return cloudflare.SetSchedule(auth.AccountID, auth.Token, name, cronExpr)
	})
	if err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}

	err = ui.RunWithSpinnerFullscreen("Updating secrets...", func() error {
		if err := cloudflare.SetSecret(auth.AccountID, auth.Token, name, "TIMEZONE", timezone); err != nil {
			return fmt.Errorf("TIMEZONE secret failed: %w", err)
		}

		notifConfig := buildNotificationConfigFromValues(notifType, webhookURL, notifyMode == "failure")
		if notifConfig != "" {
			if err := cloudflare.SetSecret(auth.AccountID, auth.Token, name, "NOTIFICATION_CONFIG", notifConfig); err != nil {
				return fmt.Errorf("NOTIFICATION_CONFIG secret failed: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update secrets: %w", err)
	}

	inst.Schedule = startTime
	inst.Timezone = timezone
	inst.CronExpression = cronExpr
	inst.NotificationType = notifType
	inst.UpdatedAt = time.Now().Format(time.RFC3339)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	for i, existing := range cfg.Instances {
		if existing.Name == name {
			cfg.Instances[i] = *inst
			break
		}
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	log.Info("instance updated", "name", name, "schedule", startTime, "timezone", timezone)

	_, nextLabel := schedule.GetNextTrigger(slots, timezone)

	ui.ExitAltScreen()

	fmt.Println()
	fmt.Println(ui.SuccessStyle.Render("  ✓ Update complete!"))
	fmt.Println()
	fmt.Printf("  Worker:       %s\n", name)
	fmt.Printf("  Schedule:     %s (%s)\n", startTime, timezone)
	fmt.Printf("  Next trigger: %s\n", nextLabel)
	fmt.Printf("  Console:      %s\n", cloudflare.ConsoleURL(auth.AccountID, name))
	fmt.Println()

	return nil
}

func buildNotificationConfigFromValues(notifType, webhookURL string, failureOnly bool) string {
	r := &ui.WizardResult{
		NotificationType:    notifType,
		NotificationWebhook: webhookURL,
		NotifyOnFailureOnly: failureOnly,
	}
	return r.BuildNotificationConfig()
}

func instanceNotFoundError(name string) error {
	instances := config.ListInstances()
	if len(instances) == 0 {
		return fmt.Errorf("instance %q not found. No instances configured — run `tokfresh init` first", name)
	}

	names := make([]string, len(instances))
	for i, inst := range instances {
		names[i] = inst.Name
	}
	return fmt.Errorf("instance %q not found. Available instances:\n  %s", name, strings.Join(names, "\n  "))
}

func resolveCloudflareAuthAuto() (*cloudflare.AuthResult, error) {
	if result := cloudflare.ResolveFromEnv(); result != nil {
		return result, nil
	}

	if cloudflare.IsWranglerInstalled() {
		if token, err := cloudflare.GetWranglerToken(); err == nil {
			return &cloudflare.AuthResult{Token: token, Source: "wrangler"}, nil
		}
	}

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

func ensureAuthForInstance(inst *config.Instance) (*cloudflare.AuthResult, error) {
	auth, err := resolveCloudflareAuthAuto()
	if err != nil {
		return nil, err
	}

	if auth.AccountID == "" {
		auth.AccountID = inst.CloudflareAccountID
	}

	if err := cloudflare.EnsureAccountAccess(auth.Token, inst.CloudflareAccountID); err != nil {
		if errors.Is(err, cloudflare.ErrUnauthorized) {
			accountLabel := inst.CloudflareAccountName
			if accountLabel == "" {
				accountLabel = inst.CloudflareAccountID
			}

			mismatchMsg := fmt.Sprintf("This worker belongs to account '%s'", accountLabel)
			if inst.CloudflareEmail != "" {
				mismatchMsg += fmt.Sprintf(" (%s)", inst.CloudflareEmail)
			}
			mismatchMsg += " but your current credentials cannot access it."

			fmt.Println()
			fmt.Printf("  %s\n", ui.ErrorStyle.Render("✗ "+mismatchMsg))
			fmt.Println()

			if auth.Source == "env" {
				fmt.Println(ui.MutedStyle.Render("  Set CLOUDFLARE_API_TOKEN to a token for that account and try again."))
				fmt.Println()
				return nil, fmt.Errorf("account access denied for '%s'", accountLabel)
			}

			for {
				switchAccount := false
				form := huh.NewForm(huh.NewGroup(
					huh.NewConfirm().
						Title("Switch to the correct account?").
						Description(mismatchMsg).
						Affirmative("Yes").
						Negative("No").
						Value(&switchAccount),
				))
				form.WithTheme(ui.TokFreshTheme())
				if err := form.Run(); err != nil {
					return nil, fmt.Errorf("cancelled")
				}

				if !switchAccount {
					switch auth.Source {
					case "wrangler":
						fmt.Println(ui.MutedStyle.Render("  Switch to the correct account and try again:"))
						fmt.Println(ui.MutedStyle.Render("    wrangler login"))
					default:
						fmt.Println(ui.MutedStyle.Render("  Provide a token with access to that account."))
					}
					fmt.Println()
					return nil, fmt.Errorf("account access denied for '%s'", accountLabel)
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
					return nil, fmt.Errorf("wrangler login failed: %w", loginErr)
				}
				ui.EnterAltScreen()

				token, tokenErr := cloudflare.GetWranglerToken()
				if tokenErr != nil {
					return nil, fmt.Errorf("wrangler token retrieval failed: %w", tokenErr)
				}
				auth.Token = token
				auth.Source = "wrangler"

				if accessErr := cloudflare.EnsureAccountAccess(auth.Token, inst.CloudflareAccountID); accessErr != nil {
					if errors.Is(accessErr, cloudflare.ErrUnauthorized) {
						fmt.Println()
						fmt.Printf("  %s\n", ui.ErrorStyle.Render("✗ Still cannot access the required account. Try again with a different account."))
						fmt.Println()
						continue
					}
					return nil, accessErr
				}

				if inst.CloudflareEmail == "" {
					if verifyResult, verifyErr := cloudflare.VerifyToken(auth.Token); verifyErr == nil && verifyResult.Email != "" {
						if cfg, loadErr := config.Load(); loadErr == nil {
							for i := range cfg.Instances {
								if cfg.Instances[i].Name == inst.Name {
									cfg.Instances[i].CloudflareEmail = verifyResult.Email
									if saveErr := config.Save(cfg); saveErr != nil {
										log.Debug("failed to backfill email in config", "error", saveErr)
									}
									break
								}
							}
						}
					}
				}

				break
			}
		} else {
			return nil, err
		}
	}

	return auth, nil
}
