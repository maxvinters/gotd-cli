package pretty

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	fn()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatal(err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatal(err)
	}
	_ = stdoutR.Close()
	_ = stderrR.Close()
	return string(stdoutBytes), string(stderrBytes)
}

func TestMiddlewareLogsTypesOnlyToStderr(t *testing.T) {
	const (
		loginToken = "BOT_TOKEN_PRIVATE_SENTINEL"
		userName   = "PRIVATE_USER_SENTINEL"
		botName    = "PRIVATE_BOT_SENTINEL"
	)
	request := &tg.AuthImportLoginTokenRequest{Token: []byte(loginToken)}
	response := &tg.User{Bot: true}
	response.SetFirstName(userName)
	response.SetUsername(botName)
	stdout, stderr := captureStreams(t, func() {
		invoke := Middleware()(telegram.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error { return nil }))
		if err := invoke(context.Background(), request, response); err != nil {
			t.Fatal(err)
		}
	})
	logs := stdout + stderr
	for _, secret := range []string{loginToken, base64.StdEncoding.EncodeToString([]byte(loginToken)), userName, botName} {
		if strings.Contains(logs, secret) {
			t.Fatalf("debug log exposed %q: %s", secret, logs)
		}
	}
	if stdout != "" {
		t.Fatalf("debug middleware wrote to stdout: %q", stdout)
	}
	for _, typeName := range []string{"AuthImportLoginTokenRequest", "User"} {
		if !strings.Contains(stderr, typeName) {
			t.Fatalf("debug log omits request/response type %q: %s", typeName, stderr)
		}
	}
}

func TestMiddlewareLogsRPCCodeWithoutErrorText(t *testing.T) {
	const privateError = "PRIVATE_RPC_PAYLOAD_SENTINEL"
	stdout, stderr := captureStreams(t, func() {
		invoke := Middleware()(telegram.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
			return tgerr.New(401, privateError)
		}))
		_ = invoke(context.Background(), &tg.AuthImportLoginTokenRequest{}, &tg.User{})
	})
	logs := stdout + stderr
	if strings.Contains(logs, privateError) {
		t.Fatalf("debug log exposed RPC error text: %s", logs)
	}
	if stdout != "" {
		t.Fatalf("debug middleware wrote to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "error_code=401") {
		t.Fatalf("debug log omits RPC error code: %s", stderr)
	}
}
