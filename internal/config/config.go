package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/charmbracelet/log"
)

var validWorkerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,61}[A-Za-z0-9]$`)

func ValidateWorkerName(name string) error {
	if len(name) < 2 {
		return fmt.Errorf("worker name must be at least 2 characters")
	}
	if len(name) > 63 {
		return fmt.Errorf("worker name must be 63 characters or less")
	}
	if !validWorkerName.MatchString(name) {
		return fmt.Errorf("worker name can only contain letters, digits, and hyphens (no underscores or leading/trailing hyphens)")
	}
	return nil
}

// Instance represents a single TokFresh worker deployment.
type Instance struct {
	Name                  string `json:"name"`
	Label                 string `json:"label,omitempty"`
	CloudflareAccountID   string `json:"cloudflareAccountId"`
	CloudflareAccountName string `json:"cloudflareAccountName,omitempty"`
	CloudflareEmail       string `json:"cloudflareEmail,omitempty"`
	Schedule              string `json:"schedule"`
	Timezone              string `json:"timezone"`
	CronExpression        string `json:"cronExpression"`
	NotificationType      string `json:"notificationType,omitempty"`
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
}

// Config represents the local TokFresh configuration file.
type Config struct {
	Instances                  []Instance `json:"instances"`
	DefaultCloudflareAccountID string     `json:"defaultCloudflareAccountId,omitempty"`
	SubscribedEmail            string     `json:"subscribedEmail,omitempty"`
}

// configDir is the directory for config files. It is a var so tests can override it.
var configDir string

// SetConfigDir overrides the config directory (used in tests).
func SetConfigDir(dir string) {
	configDir = dir
}

// Dir returns the config directory path (~/.tokfresh).
func Dir() string {
	if configDir != "" {
		return configDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Warn("cannot determine home directory, using current directory", "error", err)
		return ".tokfresh"
	}
	return filepath.Join(home, ".tokfresh")
}

// Path returns the full path to the config file.
func Path() string {
	return filepath.Join(Dir(), "config.json")
}

// Load reads and parses the config file. Returns an empty Config if the file doesn't exist.
func Load() (*Config, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the config to disk. Uses atomic write (write tmp → rename).
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmpPath := Path() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Rename(tmpPath, Path()); err != nil {
		// Clean up temp file on rename failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save config: %w", err)
	}

	log.Debug("config saved", "path", Path())
	return nil
}

// AddInstance adds a new instance to the config and saves it.
func AddInstance(inst Instance) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	// Set timestamps
	now := time.Now().Format(time.RFC3339)
	inst.CreatedAt = now
	inst.UpdatedAt = now

	cfg.Instances = append(cfg.Instances, inst)

	// Set default account ID if this is the first instance
	if cfg.DefaultCloudflareAccountID == "" && inst.CloudflareAccountID != "" {
		cfg.DefaultCloudflareAccountID = inst.CloudflareAccountID
	}

	return Save(cfg)
}

// RemoveInstance removes an instance by name and saves the config.
func RemoveInstance(name string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	found := false
	filtered := make([]Instance, 0, len(cfg.Instances))
	for _, inst := range cfg.Instances {
		if inst.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, inst)
	}

	if !found {
		return fmt.Errorf("instance %q not found", name)
	}

	cfg.Instances = filtered
	return Save(cfg)
}

// GetInstance returns an instance by name, or nil if not found.
func GetInstance(name string) *Instance {
	cfg, err := Load()
	if err != nil {
		return nil
	}

	for i := range cfg.Instances {
		if cfg.Instances[i].Name == name {
			return &cfg.Instances[i]
		}
	}
	return nil
}

// ListInstances returns all configured instances.
func ListInstances() []Instance {
	cfg, err := Load()
	if err != nil {
		return nil
	}
	return cfg.Instances
}

func GenerateWorkerName() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("tokfresh-scheduler-%s", hex.EncodeToString(b))
}
