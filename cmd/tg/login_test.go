package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTermAuthPresetValues(t *testing.T) {
	ta := termAuth{phone: "+100", password: "secret"}
	ctx := context.Background()

	if got, err := ta.Phone(ctx); err != nil || got != "+100" {
		t.Errorf("Phone() = %q, %v", got, err)
	}
	if got, err := ta.Password(ctx); err != nil || got != "secret" {
		t.Errorf("Password() = %q, %v", got, err)
	}
}

func TestTermAuthPasswordUsesNoEchoTerminalReader(t *testing.T) {
	var out bytes.Buffer
	var gotFD int
	ta := termAuth{
		in:            bufio.NewReader(strings.NewReader("unused\n")),
		out:           &out,
		passwordInput: os.Stdin,
		isTerminal:    func(int) bool { return true },
		readPassword: func(fd int) ([]byte, error) {
			gotFD = fd
			return []byte("hidden"), nil
		},
	}
	got, err := ta.Password(context.Background())
	if err != nil || got != "hidden" {
		t.Fatalf("Password() = %q, %v", got, err)
	}
	if gotFD != int(os.Stdin.Fd()) {
		t.Fatalf("ReadPassword fd = %d, want stdin fd %d", gotFD, os.Stdin.Fd())
	}
	if out.String() != "2FA password: \n" {
		t.Fatalf("password prompt output = %q", out.String())
	}
}

func TestTermAuthPrompts(t *testing.T) {
	var out bytes.Buffer
	ta := termAuth{
		in:  bufio.NewReader(strings.NewReader("+199\n123456\n")),
		out: &out,
	}
	ctx := context.Background()

	phone, err := ta.Phone(ctx)
	if err != nil || phone != "+199" {
		t.Fatalf("Phone() = %q, %v", phone, err)
	}
	code, err := ta.Code(ctx, nil)
	if err != nil || code != "123456" {
		t.Fatalf("Code() = %q, %v", code, err)
	}
	if !strings.Contains(out.String(), "Phone") || !strings.Contains(out.String(), "Code") {
		t.Errorf("prompts not written to out: %q", out.String())
	}
}

func TestTermAuthSignUpUnsupported(t *testing.T) {
	if _, err := (termAuth{}).SignUp(context.Background()); err == nil {
		t.Error("expected SignUp to be unsupported")
	}
}

func TestLoginRejectsPasswordArgvBeforeWritingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	a := &app{configPath: path, cfg: Config{}}
	cmd := a.newLoginCmd()
	if err := cmd.Flags().Set("password", "do-not-print-this"); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("login accepted a password passed in argv")
	}
	if strings.Contains(err.Error(), "do-not-print-this") {
		t.Fatal("login echoed password in error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("login wrote config before rejecting argv password: stat err = %v", err)
	}
}

func TestEnsureAccountRejectsUnsafeLabelBeforeWritingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	a := &app{configPath: path, accountFlag: "../outside", cfg: Config{}}
	if err := a.ensureAccount(); err == nil {
		t.Fatal("ensureAccount accepted a path-like label")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("ensureAccount wrote config for unsafe label: stat err = %v", err)
	}
}
