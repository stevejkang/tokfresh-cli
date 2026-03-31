package cloudflare_test

import (
	"testing"

	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
)

func TestDetectAuthState_EnvVar(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-token")
	state := cloudflare.DetectAuthState()
	if state != cloudflare.AuthFromEnv {
		t.Errorf("expected AuthFromEnv, got %v", state)
	}
}

func TestResolveFromEnv(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok123")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc456")
	result := cloudflare.ResolveFromEnv()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Token != "tok123" {
		t.Errorf("expected token tok123, got %s", result.Token)
	}
	if result.AccountID != "acc456" {
		t.Errorf("expected accountID acc456, got %s", result.AccountID)
	}
	if result.Source != "env" {
		t.Errorf("expected source env, got %s", result.Source)
	}
}

func TestResolveFromEnv_NotSet(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	result := cloudflare.ResolveFromEnv()
	if result != nil {
		t.Error("expected nil result when env var not set")
	}
}

func TestDetectAuthState_NoEnvNoWrangler(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	t.Setenv("PATH", "/nonexistent")
	state := cloudflare.DetectAuthState()
	// With no env var and no wrangler on path, should be WranglerMissing
	if state != cloudflare.AuthWranglerMissing {
		t.Errorf("expected AuthWranglerMissing, got %v", state)
	}
}

func TestAuthStateString(t *testing.T) {
	tests := []struct {
		state cloudflare.AuthState
		want  string
	}{
		{cloudflare.AuthFromEnv, "env"},
		{cloudflare.AuthFromWrangler, "wrangler"},
		{cloudflare.AuthWranglerNeedsLogin, "wrangler-needs-login"},
		{cloudflare.AuthWranglerMissing, "wrangler-missing"},
	}
	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("AuthState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestIsWranglerInstalled(t *testing.T) {
	// This test is environment-dependent. We just check it doesn't panic.
	_ = cloudflare.IsWranglerInstalled()
}
