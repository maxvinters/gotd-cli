//go:build darwin

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
)

type fakeKeychain struct {
	item    []byte
	badRead bool
	failAdd bool
}

func (f *fakeKeychain) run(_ context.Context, req keychainRequest) ([]byte, error) {
	switch req.operation {
	case keychainAddOperation:
		if f.failAdd {
			return nil, errors.New("add failed")
		}
		f.item = []byte(req.password)
		if f.badRead {
			f.item = []byte("wrong")
		}
		return nil, nil
	case keychainFindOperation:
		if f.item == nil {
			return nil, errKeychainNotFound
		}
		return append(append([]byte(nil), f.item...), '\n'), nil
	case keychainDelOperation:
		f.item = nil
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected operation %q", req.operation)
	}
}

func TestKeychainCommandQuotingRejectsInjection(t *testing.T) {
	line, err := keychainLine(keychainRequest{
		operation: keychainAddOperation,
		service:   `service"\\`,
		account:   "account",
		password:  "session-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, `service\"\\\\`) || !strings.Contains(line, `"session-secret"`) {
		t.Fatalf("security command did not quote arguments safely: %q", line)
	}
	_, err = keychainLine(keychainRequest{
		operation: keychainAddOperation,
		service:   "service\n\"delete-generic-password\"",
		account:   `account\\"`,
		password:  "secret",
	})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("newline-injected command error = %v; want safe rejection", err)
	}
}

func TestKeychainMigrationUsesFakeExecutor(t *testing.T) {
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	fake := &fakeKeychain{}
	store := &keychainStore{service: "scoped-service", account: "migrate", legacy: legacy, run: fake.run}
	want := []byte(`{"v":1,"data":"abc=="}`)
	if err := os.WriteFile(legacy, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.Exists(ctx); err != nil || !ok {
		t.Fatalf("Exists() with legacy file = %v, %v; want true, nil", ok, err)
	}
	got, err := store.LoadSession(ctx)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("LoadSession() = %q, %v; want %q, nil", got, err, want)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy file should remain after automatic migration, stat err = %v", err)
	}
	got, err = store.LoadSession(ctx)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("LoadSession() after migration = %q, %v; want %q, nil", got, err, want)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSession(ctx); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("LoadSession() after delete = %v; want ErrNotFound", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("explicit Delete should remove legacy session, stat err = %v", err)
	}
}

func TestKeychainWriteVerifiesReadbackAndKeepsLegacyOnFailure(t *testing.T) {
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	want := []byte("legacy session")
	if err := os.WriteFile(legacy, want, 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &fakeKeychain{badRead: true}
	store := &keychainStore{service: "scoped-service", account: "account", legacy: legacy, run: fake.run}
	if err := store.StoreSession(ctx, []byte("new session")); err == nil {
		t.Fatal("StoreSession accepted a mismatched readback")
	}
	got, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("legacy session = %q, %v; want preserved", got, err)
	}
}

func TestKeychainWritePreservesConcurrentLegacyReplacement(t *testing.T) {
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(legacy, []byte("old legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &fakeKeychain{}
	replacement := []byte("replacement from concurrent process")
	store := &keychainStore{
		service: "scoped-service",
		account: "account",
		legacy:  legacy,
		run: func(ctx context.Context, req keychainRequest) ([]byte, error) {
			out, err := fake.run(ctx, req)
			if err == nil && req.operation == keychainAddOperation {
				if writeErr := os.WriteFile(legacy, replacement, 0o600); writeErr != nil {
					return nil, writeErr
				}
			}
			return out, err
		},
	}
	if err := store.StoreSession(ctx, []byte("new Keychain session")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("legacy file = %q, %v; want concurrent replacement preserved", got, err)
	}
}

func TestKeychainPayloadFitsInteractiveCommandAndRoundTripsTypicalSession(t *testing.T) {
	ctx := context.Background()
	mem := &session.StorageMemory{}
	loader := &session.Loader{Storage: mem}
	data := &session.Data{
		Config: session.Config{ThisDC: 2, Date: 100, Expires: 200, DCTxtDomainName: "apv3.stel.com"},
		DC:     2,
		Addr:   "149.154.167.51:443",
		Salt:   987654321,
	}
	data.AuthKey = make([]byte, 256)
	data.AuthKeyID = make([]byte, 8)
	if _, err := rand.Read(data.AuthKey); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(data.AuthKeyID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		data.Config.DCOptions = append(data.Config.DCOptions, tg.DCOption{
			ID:        i + 1,
			IPAddress: fmt.Sprintf("149.154.%d.%d", 100+i/4, 10+i),
			Port:      443,
		})
	}
	if err := loader.Save(ctx, data); err != nil {
		t.Fatal(err)
	}
	want, err := mem.LoadSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeKeychainSession(want)
	if err != nil {
		t.Fatal(err)
	}
	line, err := keychainLine(keychainRequest{
		operation: keychainAddOperation,
		service:   "svc",
		account:   "acct",
		password:  string(payload),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(line) > maxSecurityCommandLen {
		t.Fatalf("typical session command length = %d, limit %d", len(line), maxSecurityCommandLen)
	}
	got, err := decodeKeychainSession(payload)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("decoded session = %d bytes, %v; want %d bytes", len(got), err, len(want))
	}
	gotData, err := (&session.Loader{Storage: &singleSession{data: got}}).Load(ctx)
	if err != nil || !reflect.DeepEqual(gotData, data) {
		t.Fatalf("session loader round-trip mismatch: err=%v", err)
	}
}

func TestEncodeKeychainSessionRejectsOversizedRawSession(t *testing.T) {
	if _, err := encodeKeychainSession(make([]byte, maxKeychainSessionLen+1)); err == nil {
		t.Fatal("encodeKeychainSession accepted raw session larger than decode limit")
	}
}

func TestKeychainStoreSurfacesInteractiveCommandLimit(t *testing.T) {
	data := make([]byte, 5<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	called := false
	store := &keychainStore{
		service: "service",
		account: "account",
		run: func(context.Context, keychainRequest) ([]byte, error) {
			called = true
			return nil, nil
		},
	}
	err := store.StoreSession(context.Background(), data)
	if err == nil || !strings.Contains(err.Error(), "keychain: false") {
		t.Fatalf("StoreSession error = %v; want actionable file-storage guidance", err)
	}
	if called {
		t.Fatal("StoreSession invoked Keychain executor for oversized command")
	}
}

type singleSession struct{ data []byte }

func (s *singleSession) LoadSession(context.Context) ([]byte, error) { return s.data, nil }
func (*singleSession) StoreSession(context.Context, []byte) error    { return nil }

func TestKeychainStoreUsesScopedServiceWithoutGlobalMigration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	store, ok := keychainSessionStore(keychainService(configPath), "acct", filepath.Join(filepath.Dir(configPath), "legacy.json"))
	if !ok {
		t.Fatal("Keychain backend unavailable on darwin")
	}
	if store.(*keychainStore).service == sessionService {
		t.Fatal("new store still queries the old global Keychain service")
	}
}

// exitErr fabricates an *exec.ExitError with the given code without touching Keychain.
func exitErr(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+itoa(code)).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != code {
		t.Fatalf("could not fabricate exit %d: %v", code, err)
	}
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestIsNotFound(t *testing.T) {
	if !isNotFound(exitErr(t, keychainNotFound)) {
		t.Fatal("exit 44 should be not-found")
	}
	if isNotFound(exitErr(t, 1)) {
		t.Fatal("exit 1 should not be not-found")
	}
	if isNotFound(nil) {
		t.Fatal("nil should not be not-found")
	}
	if isNotFound(errors.New("plain")) {
		t.Fatal("non-exit error should not be not-found")
	}
}
