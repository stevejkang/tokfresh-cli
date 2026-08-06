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

	// Check for dynamic model fetching
	if !strings.Contains(code, "TOKFRESH_BASE = 'https://tokfresh.com'") {
		t.Error("missing TOKFRESH_BASE constant")
	}

	if !strings.Contains(code, "/api/config/model") {
		t.Error("missing model config endpoint")
	}

	if !strings.Contains(code, "claude-haiku-4-5-20251001") {
		t.Error("missing fallback model constant")
	}

	if !strings.Contains(code, "/api/config/headers") {
		t.Error("missing headers config endpoint")
	}

	if !strings.Contains(code, "dynamicHeaders['anthropic-beta']") {
		t.Error("missing dynamic anthropic-beta header")
	}

	if !strings.Contains(code, "dynamicHeaders['x-anthropic-billing-header']") {
		t.Error("missing dynamic billing header")
	}

	if !strings.Contains(code, "cc_version=2.1.80.claude-haiku-4-5-20251001") {
		t.Error("missing fallback billing header value in FALLBACK_HEADERS")
	}

	// Ensure hardcoded model is removed
	if strings.Contains(code, "claude-sonnet-4-20250514") {
		t.Error("hardcoded claude-sonnet-4-20250514 must be removed")
	}

	// Interpolation guard — PERMANENT, must never be removed
	if strings.Contains(code, "${") {
		t.Error("template must not contain ${ interpolation (Go raw string safety)")
	}

	// Anchor invariant — GenerateTestWorkerCode relies on this prefix
	if !strings.HasPrefix(code, "export default {") {
		t.Error("template must start with 'export default {' (anchor invariant)")
	}

	// Anonymous metrics ping — exactly two occurrences (success + failure paths)
	if strings.Count(code, "/api/ping") != 2 {
		t.Errorf("expected exactly 2 occurrences of /api/ping, got %d", strings.Count(code, "/api/ping"))
	}

	// Both pings must be fire-and-forget (wrapped in try/catch)
	if strings.Count(code, "try { await fetch(TOKFRESH_BASE + '/api/ping', { method: 'POST' }); } catch {}") != 2 {
		t.Errorf("expected exactly 2 fire-and-forget ping lines, got %d", strings.Count(code, "try { await fetch(TOKFRESH_BASE + '/api/ping', { method: 'POST' }); } catch {}"))
	}
}
