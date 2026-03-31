package cloudflare_test

import (
	"strings"
	"testing"

	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
)

func TestGenerateWorkerCode(t *testing.T) {
	code := cloudflare.GenerateWorkerCode()

	if code == "" {
		t.Fatal("GenerateWorkerCode returned empty string")
	}

	// Check for scheduled handler
	if !strings.Contains(code, "async scheduled(event, env, ctx)") {
		t.Error("missing scheduled handler")
	}

	// Check for token endpoint
	if !strings.Contains(code, "console.anthropic.com/v1/oauth/token") {
		t.Error("missing token endpoint")
	}

	// Check for KV binding
	if !strings.Contains(code, "env.TOKEN_STORE") {
		t.Error("missing KV binding reference")
	}

	// Check for Claude API call
	if !strings.Contains(code, "api.anthropic.com/v1/messages") {
		t.Error("missing Claude API endpoint")
	}

	// Check for client_id
	if !strings.Contains(code, "9d1c250a-e61b-44d9-88ed-5944d1962f5e") {
		t.Error("missing client_id")
	}

	// Check for notification support
	if !strings.Contains(code, "NOTIFICATION_CONFIG") {
		t.Error("missing notification config")
	}

	// Check for Slack webhook support
	if !strings.Contains(code, "slackWebhook") {
		t.Error("missing Slack webhook support")
	}

	// Check for Discord webhook support
	if !strings.Contains(code, "discordWebhook") {
		t.Error("missing Discord webhook support")
	}

	// Check for retry logic
	if !strings.Contains(code, "attempt < 2") {
		t.Error("missing retry logic")
	}

	// Check for refresh token rotation (KV put)
	if !strings.Contains(code, "TOKEN_STORE.put") {
		t.Error("missing refresh token rotation (KV put)")
	}

	// Check for export default (ES module format)
	if !strings.Contains(code, "export default") {
		t.Error("missing ES module export")
	}
}
