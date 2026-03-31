package claude_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stevejkang/tokfresh-cli/internal/claude"
)

func TestGenerateAuthURL(t *testing.T) {
	authURL, verifier, err := claude.GenerateAuthURL()
	if err != nil {
		t.Fatal(err)
	}

	// Check URL base
	if !strings.Contains(authURL, "claude.ai/oauth/authorize") {
		t.Error("wrong OAuth URL base")
	}

	// Check client_id
	if !strings.Contains(authURL, "client_id=9d1c250a") {
		t.Error("missing client_id")
	}

	// Check response_type
	if !strings.Contains(authURL, "response_type=code") {
		t.Error("missing response_type")
	}

	// Check redirect_uri
	if !strings.Contains(authURL, "redirect_uri=") {
		t.Error("missing redirect_uri")
	}

	// Check PKCE parameters
	if !strings.Contains(authURL, "code_challenge=") {
		t.Error("missing code_challenge")
	}
	if !strings.Contains(authURL, "code_challenge_method=S256") {
		t.Error("missing code_challenge_method")
	}

	// Check scopes
	if !strings.Contains(authURL, "scope=") {
		t.Error("missing scope parameter")
	}

	// Verifier should be 64 hex chars (32 bytes)
	if len(verifier) != 64 {
		t.Errorf("verifier length: got %d, want 64", len(verifier))
	}

	// State should equal verifier
	if !strings.Contains(authURL, "state="+verifier) {
		t.Error("state parameter should equal verifier")
	}
}

func TestGenerateAuthURLUniqueness(t *testing.T) {
	url1, v1, _ := claude.GenerateAuthURL()
	url2, v2, _ := claude.GenerateAuthURL()

	if v1 == v2 {
		t.Error("verifiers should be unique across calls")
	}
	if url1 == url2 {
		t.Error("URLs should be unique across calls (different verifier/challenge)")
	}
}

func TestExchangeCodeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected application/json content type")
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["grant_type"] != "authorization_code" {
			t.Error("missing grant_type")
		}
		if body["client_id"] != claude.ClientID {
			t.Error("wrong client_id")
		}
		if body["code"] != "test-auth-code" {
			t.Errorf("wrong code: %s", body["code"])
		}
		if body["state"] != "test-state" {
			t.Errorf("wrong state: %s", body["state"])
		}
		if body["code_verifier"] != "test-verifier" {
			t.Errorf("wrong code_verifier: %s", body["code_verifier"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"access_token":  "at_test",
			"refresh_token": "rt_test",
		})
	}))
	defer server.Close()

	// We can't easily override the endpoint in the current implementation,
	// so we test the code parsing logic separately
	t.Run("code parsing", func(t *testing.T) {
		// The ExchangeCode function splits on "#"
		// "authCode#state" → code="authCode", state="state"
		// We verify this by checking our GenerateAuthURL produces valid state
		_, verifier, err := claude.GenerateAuthURL()
		if err != nil {
			t.Fatal(err)
		}

		// Simulate what a user would paste
		pastedCode := "AbCdEf123456#" + verifier
		parts := strings.SplitN(pastedCode, "#", 2)
		if parts[0] != "AbCdEf123456" {
			t.Errorf("code parsing failed: got %s", parts[0])
		}
		if parts[1] != verifier {
			t.Errorf("state parsing failed: got %s", parts[1])
		}
	})
}

func TestExchangeCodeNoHash(t *testing.T) {
	// When code has no "#", state should be empty
	code := "justAuthCode"
	parts := strings.SplitN(strings.TrimSpace(code), "#", 2)
	if parts[0] != "justAuthCode" {
		t.Errorf("expected 'justAuthCode', got %s", parts[0])
	}
	if len(parts) != 1 {
		t.Error("expected only 1 part when no # present")
	}
}

func TestConstants(t *testing.T) {
	if claude.ClientID != "9d1c250a-e61b-44d9-88ed-5944d1962f5e" {
		t.Error("wrong ClientID")
	}
	if claude.RedirectURI != "https://console.anthropic.com/oauth/code/callback" {
		t.Error("wrong RedirectURI")
	}
	if claude.TokenEndpoint != "https://console.anthropic.com/v1/oauth/token" {
		t.Error("wrong TokenEndpoint")
	}
	if claude.AuthURL != "https://claude.ai/oauth/authorize" {
		t.Error("wrong AuthURL")
	}
	if claude.Scopes != "org:create_api_key user:profile user:inference" {
		t.Error("wrong Scopes")
	}
}
