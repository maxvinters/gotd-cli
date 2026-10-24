package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gotd/cli/internal/output"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestEmitLineEscapesTextControlsAndKeepsUnicode(t *testing.T) {
	event := watchEvent{
		Peer:    peerRef{ID: 7, Type: peerUser, Name: "Аліса\x1b[2J"},
		Message: messageItem{Text: "hello\r\nnext"},
	}
	got := captureStdout(t, func() { emitLine(output.Text, "", event) })
	if want := "Аліса\\x1b[2J: hello\\r\\nnext\n"; got != want {
		t.Fatalf("text output = %q, want %q", got, want)
	}
}

func TestEmitLineJSONRemainsValidAndPreservesValues(t *testing.T) {
	event := watchEvent{
		Peer:    peerRef{ID: 7, Type: peerUser, Name: "Аліса"},
		Message: messageItem{Text: "hello\x1b[2J\r\nnext"},
	}
	got := captureStdout(t, func() { emitLine(output.JSON, "бот", event) })
	var decoded struct {
		Account string      `json:"account"`
		Peer    peerRef     `json:"peer"`
		Message messageItem `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &decoded); err != nil {
		t.Fatalf("invalid watch JSON %q: %v", got, err)
	}
	if decoded.Account != "бот" || decoded.Peer.Name != "Аліса" || decoded.Message.Text != event.Message.Text {
		t.Fatalf("decoded event = %+v", decoded)
	}
}

func TestWaitTextMarshalerEscapesMessageControls(t *testing.T) {
	event := watchEvent{
		Peer:    peerRef{ID: 7, Type: peerUser, Name: "Аліса"},
		Message: messageItem{Text: "hello\r\nnext"},
	}
	var buf bytes.Buffer
	if err := output.New(output.Text, &buf).Emit(event); err != nil {
		t.Fatal(err)
	}
	if want := "Аліса: hello\\r\\nnext\n"; buf.String() != want {
		t.Fatalf("wait text = %q, want %q", buf.String(), want)
	}
}
