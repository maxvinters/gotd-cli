package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRejectsTokenArgv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	a := &app{configPath: path}
	cmd := newInitCmd(a)
	if err := cmd.Flags().Set("token", "do-not-print-this"); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("init accepted a bot token passed in argv")
	}
	if strings.Contains(err.Error(), "do-not-print-this") {
		t.Fatal("init echoed bot token in error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("init wrote config before rejecting argv token: stat err = %v", err)
	}
}
