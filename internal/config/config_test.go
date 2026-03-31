package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stevejkang/tokfresh-cli/internal/config"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	config.SetConfigDir(dir)
	t.Cleanup(func() { config.SetConfigDir("") })
	return dir
}

func TestLoadEmpty(t *testing.T) {
	setupTestDir(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(cfg.Instances))
	}
}

func TestSaveAndLoad(t *testing.T) {
	setupTestDir(t)

	cfg := &config.Config{
		Instances: []config.Instance{
			{
				Name:                "test-worker",
				CloudflareAccountID: "acc123",
				Schedule:            "06:00",
				Timezone:            "Asia/Seoul",
				CronExpression:      "0 21,2,7,12 * * *",
			},
		},
		DefaultCloudflareAccountID: "acc123",
	}

	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(loaded.Instances))
	}
	if loaded.Instances[0].Name != "test-worker" {
		t.Errorf("expected name test-worker, got %s", loaded.Instances[0].Name)
	}
	if loaded.DefaultCloudflareAccountID != "acc123" {
		t.Errorf("expected default account acc123, got %s", loaded.DefaultCloudflareAccountID)
	}
}

func TestAddInstance(t *testing.T) {
	setupTestDir(t)

	err := config.AddInstance(config.Instance{
		Name:                "worker1",
		CloudflareAccountID: "acc1",
		Schedule:            "06:00",
		Timezone:            "UTC",
		CronExpression:      "0 6,11,16,21 * * *",
	})
	if err != nil {
		t.Fatal(err)
	}

	instances := config.ListInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Name != "worker1" {
		t.Errorf("expected worker1, got %s", instances[0].Name)
	}
	// Timestamps should be set
	if instances[0].CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
	if instances[0].UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestAddMultipleInstances(t *testing.T) {
	setupTestDir(t)

	for _, name := range []string{"worker1", "worker2", "worker3"} {
		err := config.AddInstance(config.Instance{
			Name:                name,
			CloudflareAccountID: "acc1",
			Schedule:            "06:00",
			Timezone:            "UTC",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	instances := config.ListInstances()
	if len(instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(instances))
	}
}

func TestRemoveInstance(t *testing.T) {
	setupTestDir(t)

	// Add two instances
	config.AddInstance(config.Instance{Name: "keep", CloudflareAccountID: "acc"})
	config.AddInstance(config.Instance{Name: "remove", CloudflareAccountID: "acc"})

	if err := config.RemoveInstance("remove"); err != nil {
		t.Fatal(err)
	}

	instances := config.ListInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Name != "keep" {
		t.Errorf("expected 'keep', got %s", instances[0].Name)
	}
}

func TestRemoveInstance_NotFound(t *testing.T) {
	setupTestDir(t)

	err := config.RemoveInstance("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent instance")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestGetInstance(t *testing.T) {
	setupTestDir(t)

	config.AddInstance(config.Instance{Name: "target", CloudflareAccountID: "acc", Schedule: "08:00"})
	config.AddInstance(config.Instance{Name: "other", CloudflareAccountID: "acc", Schedule: "09:00"})

	inst := config.GetInstance("target")
	if inst == nil {
		t.Fatal("expected non-nil instance")
	}
	if inst.Schedule != "08:00" {
		t.Errorf("expected schedule 08:00, got %s", inst.Schedule)
	}
}

func TestGetInstance_NotFound(t *testing.T) {
	setupTestDir(t)

	inst := config.GetInstance("nonexistent")
	if inst != nil {
		t.Error("expected nil for nonexistent instance")
	}
}

func TestGenerateWorkerName(t *testing.T) {
	tests := []struct {
		suffix string
		prefix string
	}{
		{"my-claude", "tokfresh-scheduler-my-claude"},
		{"", "tokfresh-scheduler-"},
		{"  work  ", "tokfresh-scheduler-work"},
	}

	for _, tt := range tests {
		name := config.GenerateWorkerName(tt.suffix)
		if !strings.HasPrefix(name, tt.prefix) {
			t.Errorf("GenerateWorkerName(%q) = %q, expected prefix %q", tt.suffix, name, tt.prefix)
		}
	}
}

func TestGenerateWorkerNameRandomness(t *testing.T) {
	name1 := config.GenerateWorkerName("")
	name2 := config.GenerateWorkerName("")
	if name1 == name2 {
		t.Error("expected unique names for empty suffix")
	}
}

func TestConfigPath(t *testing.T) {
	setupTestDir(t)

	path := config.Path()
	if !strings.HasSuffix(path, "config.json") {
		t.Errorf("path should end with config.json: %s", path)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := setupTestDir(t)

	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "test", CloudflareAccountID: "acc"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Verify file exists and no temp file remains
	if _, err := os.Stat(config.Path()); err != nil {
		t.Error("config file should exist")
	}
	tmpPath := config.Path() + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after save")
	}

	// Verify permissions (should be 0600)
	info, _ := os.Stat(config.Path())
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify directory exists
	if _, err := os.Stat(filepath.Dir(config.Path())); err != nil {
		t.Error("config directory should exist")
	}
	_ = dir
}

func TestDefaultAccountIDSetOnFirstInstance(t *testing.T) {
	setupTestDir(t)

	config.AddInstance(config.Instance{
		Name:                "first",
		CloudflareAccountID: "first-acc",
	})

	cfg, _ := config.Load()
	if cfg.DefaultCloudflareAccountID != "first-acc" {
		t.Errorf("expected default account to be set from first instance, got %s", cfg.DefaultCloudflareAccountID)
	}

	// Adding second instance should not change default
	config.AddInstance(config.Instance{
		Name:                "second",
		CloudflareAccountID: "second-acc",
	})

	cfg, _ = config.Load()
	if cfg.DefaultCloudflareAccountID != "first-acc" {
		t.Errorf("default account should not change, got %s", cfg.DefaultCloudflareAccountID)
	}
}
