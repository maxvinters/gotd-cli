package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadConfigRejectsUnsafeAccountLabels(t *testing.T) {
	for _, label := range []string{"../outside", `..\\outside`, "nested/name", "line\nbreak", "tab\tlabel", "all", "default"} {
		t.Run(label, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			data := []byte("accounts:\n  \"" + label + "\": {}\n")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadConfig(path); err == nil {
				t.Fatalf("loadConfig accepted unsafe account label %q", label)
			}
		})
	}
}

func TestSaveConfigRewriteIsPrivateAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(path, Config{AppID: 1}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("rewritten config mode = %04o, want 0600", got)
	}

	target := filepath.Join(dir, "target.yaml")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := saveConfig(link, Config{AppID: 2}); err == nil {
		t.Fatal("saveConfig accepted a symlink destination")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatalf("symlink target = %q, %v; want unchanged", got, err)
	}
}

func TestLoadConfigRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	if err := os.WriteFile(target, []byte("app_id: 10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := loadConfig(link); err == nil {
		t.Fatal("loadConfig accepted a symlink")
	}
}
