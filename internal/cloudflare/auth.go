package cloudflare

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/charmbracelet/log"
)

// AuthState represents the current Cloudflare authentication status.
type AuthState int

const (
	AuthFromEnv            AuthState = iota // CLOUDFLARE_API_TOKEN env var is set
	AuthFromWrangler                        // wrangler auth token --json succeeded
	AuthWranglerNeedsLogin                  // wrangler found but not logged in
	AuthWranglerMissing                     // wrangler not installed
)

// String returns a human-readable name for the auth state.
func (s AuthState) String() string {
	switch s {
	case AuthFromEnv:
		return "env"
	case AuthFromWrangler:
		return "wrangler"
	case AuthWranglerNeedsLogin:
		return "wrangler-needs-login"
	case AuthWranglerMissing:
		return "wrangler-missing"
	default:
		return "unknown"
	}
}

// AuthResult holds the resolved authentication credentials.
type AuthResult struct {
	Token     string
	AccountID string
	Source    string // "env", "wrangler", "manual"
}

// DetectAuthState checks available auth methods without prompting.
func DetectAuthState() AuthState {
	if os.Getenv("CLOUDFLARE_API_TOKEN") != "" {
		return AuthFromEnv
	}
	if !IsWranglerInstalled() {
		return AuthWranglerMissing
	}
	if _, err := GetWranglerToken(); err == nil {
		return AuthFromWrangler
	}
	return AuthWranglerNeedsLogin
}

// ResolveFromEnv returns token from CLOUDFLARE_API_TOKEN env var if set.
func ResolveFromEnv() *AuthResult {
	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if token == "" {
		return nil
	}
	log.Info("using CLOUDFLARE_API_TOKEN env var")
	return &AuthResult{
		Token:     token,
		AccountID: os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		Source:    "env",
	}
}

// IsWranglerInstalled checks if wrangler is on PATH.
func IsWranglerInstalled() bool {
	_, err := exec.LookPath("wrangler")
	return err == nil
}

// wranglerTokenOutput represents the JSON output from `wrangler auth token --json`.
// Wrangler returns three possible shapes:
//
//	{"type": "api_token", "token": "..."}
//	{"type": "oauth",     "token": "..."}
//	{"type": "api_key",   "key": "...", "email": "..."}  ← no "token" field!
type wranglerTokenOutput struct {
	Type  string `json:"type"`
	Token string `json:"token"` // present for api_token and oauth
	Key   string `json:"key"`   // present for api_key
	Email string `json:"email"` // present for api_key
}

// GetWranglerToken calls `wrangler auth token --json`.
// Returns a Bearer token for the CF API, or an error.
// For api_key auth: CF API uses X-Auth-Key + X-Auth-Email headers,
// but this is legacy — we reject it and suggest switching to API token.
func GetWranglerToken() (string, error) {
	log.Debug("trying wrangler auth token --json")
	cmd := exec.Command("wrangler", "auth", "token", "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("wrangler auth failed: %w", err)
	}

	var result wranglerTokenOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("failed to parse wrangler output: %w", err)
	}

	switch result.Type {
	case "api_token", "oauth":
		if result.Token == "" {
			return "", fmt.Errorf("wrangler returned empty token")
		}
		log.Debug("wrangler auth resolved", "type", result.Type)
		return result.Token, nil
	case "api_key":
		// Legacy Global API Key auth uses different HTTP headers (X-Auth-Key + X-Auth-Email).
		// Our CF client uses Bearer tokens only. Guide user to create an API token instead.
		return "", fmt.Errorf("wrangler is using a Global API Key (legacy). TokFresh requires an API token. Run `wrangler logout && wrangler login` or create a token at https://dash.cloudflare.com/profile/api-tokens")
	default:
		return "", fmt.Errorf("unknown wrangler auth type: %q", result.Type)
	}
}

// InstallWrangler attempts to install wrangler globally.
// Tries npm first, then brew on macOS.
func InstallWrangler() error {
	log.Info("installing wrangler")

	if npmPath, err := exec.LookPath("npm"); err == nil {
		log.Debug("using npm to install wrangler", "npm", npmPath)
		cmd := exec.Command("npm", "install", "-g", "wrangler")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			log.Debug("using brew to install wrangler")
			cmd := exec.Command("brew", "install", "cloudflare-wrangler2")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
	}

	return fmt.Errorf("cannot auto-install: npm not found (install Node.js or get wrangler from https://developers.cloudflare.com/workers/wrangler/install-and-update/)")
}

// RunWranglerLogin runs `wrangler login` interactively (needs TTY).
func RunWranglerLogin() error {
	log.Info("running wrangler login")
	cmd := exec.Command("wrangler", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
