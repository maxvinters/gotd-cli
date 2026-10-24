package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUseKeychain(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name string
		val  *bool
		want bool
	}{
		{"unset defaults on", nil, true},
		{"explicit on", &on, true},
		{"explicit off", &off, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &app{cfg: Config{Keychain: c.val}}
			if got := a.useKeychain(); got != c.want {
				t.Fatalf("useKeychain() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestKeychainAccount(t *testing.T) {
	acc := Account{AppID: 123}
	got := keychainAccount("work", acc, kindBot)
	want := "work." + kindBot + "." + acc.seed("work", kindBot)
	if got != want {
		t.Fatalf("keychainAccount() = %q, want %q", got, want)
	}
	// user vs bot must not collide.
	if keychainAccount("work", acc, kindUser) == got {
		t.Fatal("user and bot accounts collide")
	}
}

func TestFileSessionStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	acc := Account{AppID: 42}
	// keychain off → file backend regardless of platform.
	configPath := filepath.Join(dir, "config.yaml")
	store := newSessionStore(configPath, defaultAccount, acc, kindUser, false)

	if ok, err := store.Exists(ctx); err != nil || ok {
		t.Fatalf("Exists() = %v, %v; want false, nil", ok, err)
	}
	if err := store.StoreSession(ctx, []byte("blob")); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}
	path := acc.sessionPath(filepath.Dir(configPath), defaultAccount, kindUser)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreSession(ctx, []byte("blob")); err != nil {
		t.Fatalf("StoreSession rewrite: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("rewritten session mode = %04o, want 0600", got)
	}
	if ok, err := store.Exists(ctx); err != nil || !ok {
		t.Fatalf("Exists() after store = %v, %v; want true, nil", ok, err)
	}
	got, err := store.LoadSession(ctx)
	if err != nil || string(got) != "blob" {
		t.Fatalf("LoadSession() = %q, %v; want \"blob\", nil", got, err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := store.Exists(ctx); ok {
		t.Fatal("Exists() after delete = true, want false")
	}
	// Delete is idempotent.
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	// File lands at the expected path.
	if _, err := os.Stat(path); err == nil {
		t.Fatal("session file should be gone after delete")
	}
}

func TestFileSessionStoreRejectsSymlink(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	acc := Account{AppID: 42}
	path := acc.sessionPath(dir, defaultAccount, kindUser)
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := newSessionStore(filepath.Join(dir, "config.yaml"), defaultAccount, acc, kindUser, false)
	if err := store.StoreSession(ctx, []byte("overwrite")); err == nil {
		t.Fatal("StoreSession accepted a symlink destination")
	}
	if _, err := store.LoadSession(ctx); err == nil {
		t.Fatal("LoadSession followed a symlink")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatalf("symlink target = %q, %v; want unchanged", got, err)
	}
}

func TestKeychainServiceScopesFullConfigPath(t *testing.T) {
	dir := t.TempDir()
	first := keychainService(filepath.Join(dir, "one.yaml"))
	second := keychainService(filepath.Join(dir, "two.yaml"))
	if first == sessionService || second == sessionService {
		t.Fatal("keychain service still uses the old global name")
	}
	if first == second {
		t.Fatal("distinct config files share a Keychain service")
	}
	if first != keychainService(filepath.Join(dir, ".", "one.yaml")) {
		t.Fatal("equivalent config paths produced different Keychain services")
	}
}
