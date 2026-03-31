package cloudflare

import (
	"fmt"

	"github.com/charmbracelet/log"
)

// DeployParams holds all parameters needed for a full worker deployment.
type DeployParams struct {
	Token              string
	AccountID          string
	WorkerName         string
	WorkerCode         string
	RefreshToken       string
	CronExpression     string
	Timezone           string
	NotificationConfig string // optional JSON: {"slackWebhook":"...","failureOnly":true}
	OnProgress         func(step string)
}

// Deploy orchestrates the full worker deployment sequence:
//  1. Find/create KV namespace (per-worker isolation)
//  2. Write refresh token to KV (initial value)
//  3. Upload worker with KV binding
//  4. Set cron schedule
//  5. Store secrets (REFRESH_TOKEN, TIMEZONE, optional NOTIFICATION_CONFIG)
func Deploy(params DeployParams) error {
	log.Info("starting deployment", "worker", params.WorkerName)
	progress := params.OnProgress
	if progress == nil {
		progress = func(string) {}
	}

	// 1. Find/create KV namespace (per-worker isolation)
	progress("Creating KV namespace...")
	kvTitle := fmt.Sprintf("tokfresh-tokens-%s", params.WorkerName)
	nsID, err := FindOrCreateKV(params.AccountID, params.Token, kvTitle)
	if err != nil {
		return fmt.Errorf("KV namespace setup failed: %w", err)
	}
	log.Debug("KV namespace ready", "id", nsID, "title", kvTitle)

	// 2. Write refresh token to KV (initial value)
	progress("Storing token...")
	if err := WriteKVValue(params.AccountID, params.Token, nsID, "refresh_token", params.RefreshToken); err != nil {
		return fmt.Errorf("KV token write failed: %w", err)
	}

	// 3. Upload worker with KV binding
	progress("Deploying worker...")
	if err := UploadWorker(params.AccountID, params.Token, params.WorkerName, params.WorkerCode, nsID); err != nil {
		return fmt.Errorf("worker upload failed: %w", err)
	}

	// 4. Set cron schedule
	progress("Setting schedule...")
	if err := SetSchedule(params.AccountID, params.Token, params.WorkerName, params.CronExpression); err != nil {
		return fmt.Errorf("cron setup failed: %w", err)
	}

	// 5. Store secrets (REFRESH_TOKEN as fallback, TIMEZONE, optional NOTIFICATION_CONFIG)
	progress("Storing secrets...")
	secrets := map[string]string{
		"REFRESH_TOKEN": params.RefreshToken,
		"TIMEZONE":      params.Timezone,
	}
	if params.NotificationConfig != "" {
		secrets["NOTIFICATION_CONFIG"] = params.NotificationConfig
	}
	for name, value := range secrets {
		if err := SetSecret(params.AccountID, params.Token, params.WorkerName, name, value); err != nil {
			return fmt.Errorf("secret %q failed: %w", name, err)
		}
	}

	log.Info("deployment complete", "worker", params.WorkerName)
	return nil
}
