//go:build darwin

package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/go-faster/errors"

	"github.com/gotd/cli/internal/privatefile"
	"github.com/gotd/td/session"
)

// security(1)'s interactive parser reads one command into a 4096-byte buffer.
const (
	keychainNotFound      = 44
	maxSecurityCommandLen = 4094
	maxKeychainSessionLen = 1 << 20
	keychainSessionV1     = "v1:"
	keychainAddOperation  = "add-generic-password"
	keychainFindOperation = "find-generic-password"
	keychainDelOperation  = "delete-generic-password"
)

var errKeychainNotFound = errors.New("keychain item not found")

type keychainRequest struct {
	operation string
	service   string
	account   string
	password  string
}

type keychainCommandRunner func(context.Context, keychainRequest) ([]byte, error)

// keychainSessionStore returns a Keychain backend. Only the scoped legacy file
// is migrated; old global Keychain items have ambiguous config ownership.
func keychainSessionStore(service, account, legacy string) (sessionStore, bool) {
	return &keychainStore{service: service, account: account, legacy: legacy, run: runSecurityCommand}, true
}

// keychainStore stores a gotd session as a generic password on macOS.
type keychainStore struct {
	service string
	account string
	legacy  string
	run     keychainCommandRunner
}

func keychainLine(req keychainRequest) (string, error) {
	args := []string{req.operation, "-s", req.service, "-a", req.account}
	switch req.operation {
	case keychainAddOperation:
		args = append(args, "-U", "-w", req.password)
	case keychainFindOperation:
		args = append(args, "-w")
	case keychainDelOperation:
	default:
		return "", errors.New("unsupported Keychain operation")
	}
	for i, arg := range args {
		if strings.IndexFunc(arg, unicode.IsControl) >= 0 {
			return "", errors.New("invalid control character in Keychain command")
		}
		arg = strings.ReplaceAll(arg, `\`, `\\`)
		arg = strings.ReplaceAll(arg, `"`, `\"`)
		args[i] = `"` + arg + `"`
	}
	line := strings.Join(args, " ")
	if len(line) > maxSecurityCommandLen {
		return "", errors.New("session is too large for the Keychain command; set keychain: false to use private file storage")
	}
	return line, nil
}

func runSecurityCommand(ctx context.Context, req keychainRequest) ([]byte, error) {
	line, err := keychainLine(req)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "-q", "-p", "")
	cmd.Stdin = strings.NewReader(line + "\n")
	cmd.Stderr = io.Discard
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func isNotFound(err error) bool {
	if errors.Is(err, errKeychainNotFound) {
		return true
	}
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == keychainNotFound
}

func (s *keychainStore) runCommand(ctx context.Context, req keychainRequest) ([]byte, error) {
	if s.run == nil {
		return runSecurityCommand(ctx, req)
	}
	return s.run(ctx, req)
}

func (s *keychainStore) find(ctx context.Context) ([]byte, error) {
	out, err := s.runCommand(ctx, keychainRequest{
		operation: keychainFindOperation,
		service:   s.service,
		account:   s.account,
	})
	if err != nil {
		if isNotFound(err) {
			return nil, session.ErrNotFound
		}
		return nil, errors.New("keychain find failed")
	}
	if len(out) == 0 {
		return nil, session.ErrNotFound
	}
	if bytes.HasSuffix(out, []byte("\n")) {
		out = bytes.TrimSuffix(out, []byte("\n"))
	}
	return decodeKeychainSession(out)
}

func (s *keychainStore) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := s.find(ctx)
	if !errors.Is(err, session.ErrNotFound) {
		return data, err
	}
	data, err = privatefile.Read(s.legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "read legacy session")
	}
	if err := s.StoreSession(ctx, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *keychainStore) StoreSession(ctx context.Context, data []byte) error {
	payload, err := encodeKeychainSession(data)
	if err != nil {
		return err
	}
	if _, err := keychainLine(keychainRequest{
		operation: keychainAddOperation,
		service:   s.service,
		account:   s.account,
		password:  string(payload),
	}); err != nil {
		return err
	}
	if _, err := s.runCommand(ctx, keychainRequest{
		operation: keychainAddOperation,
		service:   s.service,
		account:   s.account,
		password:  string(payload),
	}); err != nil {
		return errors.New("keychain add failed")
	}
	got, err := s.find(ctx)
	if err != nil || !bytes.Equal(got, data) {
		return errors.New("keychain write verification failed")
	}
	// Keep the legacy file during automatic migration: another CLI process may
	// replace it while this Keychain write is in flight. `logout` removes both;
	// manual cleanup is safe after all CLI processes have stopped.
	return nil
}

func (s *keychainStore) Exists(ctx context.Context) (bool, error) {
	_, err := s.find(ctx)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, session.ErrNotFound) {
		return false, err
	}
	if _, err := privatefile.Read(s.legacy); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, errors.Wrap(err, "check legacy session")
	}
}

func (s *keychainStore) Delete(ctx context.Context) error {
	if _, err := s.runCommand(ctx, keychainRequest{
		operation: keychainDelOperation,
		service:   s.service,
		account:   s.account,
	}); err != nil && !isNotFound(err) {
		return errors.New("keychain delete failed")
	}
	if _, err := s.find(ctx); err == nil {
		return errors.New("keychain delete verification failed")
	} else if !errors.Is(err, session.ErrNotFound) {
		return err
	}
	if err := os.Remove(s.legacy); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove legacy session")
	}
	return nil
}

func encodeKeychainSession(data []byte) ([]byte, error) {
	if len(data) > maxKeychainSessionLen {
		return nil, errors.New("Keychain session exceeds 1 MiB limit")
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(data); err != nil {
		return nil, errors.Wrap(err, "compress session")
	}
	if err := zw.Close(); err != nil {
		return nil, errors.Wrap(err, "compress session")
	}
	encoded := base64.RawStdEncoding.EncodeToString(compressed.Bytes())
	return []byte(keychainSessionV1 + encoded), nil
}

func decodeKeychainSession(payload []byte) ([]byte, error) {
	if !bytes.HasPrefix(payload, []byte(keychainSessionV1)) {
		return nil, errors.New("unsupported Keychain session format")
	}
	compressed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(string(payload), keychainSessionV1))
	if err != nil {
		return nil, errors.Wrap(err, "decode Keychain session")
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, errors.Wrap(err, "decompress Keychain session")
	}
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(io.LimitReader(zr, maxKeychainSessionLen+1))
	if err != nil {
		return nil, errors.Wrap(err, "decompress Keychain session")
	}
	if len(data) > maxKeychainSessionLen {
		return nil, errors.New("Keychain session exceeds 1 MiB limit")
	}
	return data, nil
}
