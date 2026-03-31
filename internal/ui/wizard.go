package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/schedule"
)

type WizardResult struct {
	WorkerName          string
	AuthCode            string
	StartTime           string
	Timezone            string
	NotificationType    string
	NotificationWebhook string
	NotifyOnFailureOnly bool
	CloudflareAuthMode  string
	CloudflareAPIToken  string
}

func (r *WizardResult) BuildNotificationConfig() string {
	if r.NotificationType == "none" || r.NotificationWebhook == "" {
		return ""
	}

	cfg := map[string]interface{}{}
	switch r.NotificationType {
	case "slack":
		cfg["slackWebhook"] = r.NotificationWebhook
	case "discord":
		cfg["discordWebhook"] = r.NotificationWebhook
	}
	if r.NotifyOnFailureOnly {
		cfg["failureOnly"] = true
	}

	data, _ := json.Marshal(cfg)
	return string(data)
}

func buildCFAuthChoices(state cloudflare.AuthState) []huh.Option[string] {
	switch state {
	case cloudflare.AuthFromEnv:
		return []huh.Option[string]{
			huh.NewOption("Use environment variable (CLOUDFLARE_API_TOKEN)", "env"),
		}
	case cloudflare.AuthFromWrangler:
		return []huh.Option[string]{
			huh.NewOption("Use wrangler (logged in ✓)", "wrangler"),
		}
	case cloudflare.AuthWranglerNeedsLogin:
		return []huh.Option[string]{
			huh.NewOption("Login with wrangler (recommended)", "wrangler-login"),
			huh.NewOption("Enter API token manually", "manual"),
		}
	case cloudflare.AuthWranglerMissing:
		return []huh.Option[string]{
			huh.NewOption("Install wrangler & login (recommended)", "wrangler-install"),
			huh.NewOption("Enter API token manually", "manual"),
		}
	default:
		return []huh.Option[string]{
			huh.NewOption("Enter API token manually", "manual"),
		}
	}
}

func BuildStartTimeOptions() []huh.Option[string] {
	opts := make([]huh.Option[string], 24)
	for h := 0; h < 24; h++ {
		startTime := fmt.Sprintf("%02d:00", h)
		slots := schedule.Calculate(startTime)
		active := slots[:schedule.ActiveTriggerCount]
		label := fmt.Sprintf("%s  →  triggers at %s", startTime, strings.Join(active, ", "))
		opts[h] = huh.NewOption(label, startTime)
	}
	return opts
}

func BuildTimezoneOptions(detected string) []huh.Option[string] {
	zones := []string{
		"Pacific/Pago_Pago",
		"Pacific/Honolulu",
		"Pacific/Marquesas",
		"America/Anchorage",
		"America/Los_Angeles",
		"America/Denver",
		"America/Chicago",
		"America/New_York",
		"America/Caracas",
		"America/St_Johns",
		"America/Sao_Paulo",
		"Atlantic/South_Georgia",
		"Atlantic/Azores",
		"Europe/London",
		"Europe/Paris",
		"Europe/Berlin",
		"Africa/Cairo",
		"Africa/Johannesburg",
		"Europe/Moscow",
		"Asia/Tehran",
		"Asia/Dubai",
		"Asia/Kabul",
		"Asia/Karachi",
		"Asia/Kolkata",
		"Asia/Kathmandu",
		"Asia/Dhaka",
		"Asia/Yangon",
		"Asia/Bangkok",
		"Asia/Singapore",
		"Asia/Shanghai",
		"Asia/Seoul",
		"Asia/Tokyo",
		"Australia/Darwin",
		"Australia/Sydney",
		"Pacific/Noumea",
		"Pacific/Auckland",
		"Pacific/Tongatapu",
		"Pacific/Kiritimati",
		"UTC",
	}

	opts := make([]huh.Option[string], 0, len(zones)+1)
	detectedFound := false

	for _, tz := range zones {
		loc, err := time.LoadLocation(tz)
		offset := "+00:00"
		if err == nil {
			_, sec := time.Now().In(loc).Zone()
			h := sec / 3600
			m := (sec % 3600) / 60
			if m < 0 {
				m = -m
			}
			offset = fmt.Sprintf("%+03d:%02d", h, m)
		}

		label := fmt.Sprintf("%s (%s)", tz, offset)
		if tz == detected {
			label += " ← detected"
			detectedFound = true
		}
		opts = append(opts, huh.NewOption(label, tz))
	}

	if !detectedFound && detected != "" {
		loc, _ := time.LoadLocation(detected)
		offset := "+00:00"
		if loc != nil {
			_, sec := time.Now().In(loc).Zone()
			h := sec / 3600
			m := (sec % 3600) / 60
			if m < 0 {
				m = -m
			}
			offset = fmt.Sprintf("%+03d:%02d", h, m)
		}
		label := fmt.Sprintf("%s (%s) ← detected", detected, offset)
		opts = append([]huh.Option[string]{huh.NewOption(label, detected)}, opts...)
	}

	return opts
}

func RunSetupWizard(detectedTimezone, authURL string) (*WizardResult, error) {
	var result WizardResult
	result.Timezone = detectedTimezone
	result.StartTime = "06:00"
	result.NotificationType = "none"

	var notifyMode string

	authState := cloudflare.DetectAuthState()
	cfAuthChoices := buildCFAuthChoices(authState)
	skipCFAuth := authState == cloudflare.AuthFromEnv || authState == cloudflare.AuthFromWrangler

	ClearAndBrand()
	fmt.Println("  " + MutedStyle.Render("Open this URL to authorize your Claude account (Cmd+Click):"))
	fmt.Println()
	fmt.Printf("  %s\n", authURL)
	fmt.Println()

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Authorization code").
				Description("Paste the code from the Claude OAuth page").
				Value(&result.AuthCode),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Worker name").
				Description("A unique name for this scheduler. Leave empty for auto-generated.").
				Placeholder("tokfresh-work, tokfresh-personal ...").
				Value(&result.WorkerName),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Start time").
				Description("When should your first daily trigger fire?").
				Options(BuildStartTimeOptions()...).
				Value(&result.StartTime).
				Height(10),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Timezone").
				Description("Your local timezone for schedule conversion").
				Options(BuildTimezoneOptions(detectedTimezone)...).
				Value(&result.Timezone).
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
				Value(&result.NotificationType),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Webhook URL").
				Description("Paste your Slack or Discord webhook URL").
				Placeholder("https://hooks.slack.com/services/...").
				Value(&result.NotificationWebhook),
			huh.NewSelect[string]().
				Title("When to notify").
				Description("Choose when you receive notifications").
				Options(
					huh.NewOption("All triggers (success + failure)", "all"),
					huh.NewOption("Failures only", "failure"),
				).
				Value(&notifyMode),
		).WithHideFunc(func() bool {
			return result.NotificationType == "none"
		}),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Cloudflare authentication").
				Description("How to authenticate with your Cloudflare account").
				Options(cfAuthChoices...).
				Value(&result.CloudflareAuthMode),
		).WithHideFunc(func() bool {
			return skipCFAuth
		}),
	)

	form.WithTheme(TokFreshTheme())

	if err := form.Run(); err != nil {
		return nil, err
	}

	result.NotifyOnFailureOnly = notifyMode == "failure"

	if skipCFAuth {
		if authState == cloudflare.AuthFromEnv {
			result.CloudflareAuthMode = "env"
		} else {
			result.CloudflareAuthMode = "wrangler"
		}
	}

	return &result, nil
}
