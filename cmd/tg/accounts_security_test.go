package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountsAddRejectsUnsafeLabelBeforeWritingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	a := &app{configPath: path, cfg: Config{}}
	cmd := a.newAccountsAddCmd()
	cmd.SetArgs([]string{"../outside"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("accounts add accepted a path-like label")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("accounts add wrote config for unsafe label: stat err = %v", err)
	}
}

func TestAccountsAddUsesBOT_TOKEN(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token-from-env")
	path := filepath.Join(t.TempDir(), "config.yaml")
	a := &app{configPath: path, cfg: Config{}}
	cmd := a.newAccountsAddCmd()
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Accounts["work"].BotToken; got != "token-from-env" {
		t.Fatalf("account bot token = %q, want BOT_TOKEN", got)
	}
}

func TestAccountsAddRejectsTokenArgv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	a := &app{configPath: path, cfg: Config{}}
	cmd := a.newAccountsAddCmd()
	if err := cmd.Flags().Set("token", "do-not-print-this"); err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"work"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("accounts add accepted a bot token passed in argv")
	}
	if strings.Contains(err.Error(), "do-not-print-this") {
		t.Fatal("accounts add echoed bot token in error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("accounts add wrote config before rejecting argv token: stat err = %v", err)
	}
}
