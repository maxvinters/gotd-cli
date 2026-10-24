package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

func TestMediaLocationDocument(t *testing.T) {
	media := &tg.MessageMediaDocument{}
	media.Document = &tg.Document{
		ID:            5,
		AccessHash:    7,
		FileReference: []byte{1, 2},
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: "report.pdf"},
		},
	}

	loc, name, err := mediaLocation(media)
	if err != nil {
		t.Fatal(err)
	}
	if name != "report.pdf" {
		t.Errorf("name = %q, want report.pdf", name)
	}
	if _, ok := loc.(*tg.InputDocumentFileLocation); !ok {
		t.Errorf("loc type = %T, want InputDocumentFileLocation", loc)
	}
}

func TestMediaLocationPhoto(t *testing.T) {
	media := &tg.MessageMediaPhoto{}
	media.Photo = &tg.Photo{
		ID:         9,
		AccessHash: 1,
		Sizes: []tg.PhotoSizeClass{
			&tg.PhotoSize{Type: "m", W: 100, H: 100},
			&tg.PhotoSize{Type: "y", W: 1000, H: 1000},
		},
	}

	loc, name, err := mediaLocation(media)
	if err != nil {
		t.Fatal(err)
	}
	if name != "photo_9.jpg" {
		t.Errorf("name = %q", name)
	}
	pl, ok := loc.(*tg.InputPhotoFileLocation)
	if !ok {
		t.Fatalf("loc type = %T", loc)
	}
	if pl.ThumbSize != "y" {
		t.Errorf("picked size %q, want largest 'y'", pl.ThumbSize)
	}
}

func TestMediaLocationNone(t *testing.T) {
	if _, _, err := mediaLocation(&tg.MessageMediaEmpty{}); err == nil {
		t.Error("expected error for empty media")
	}
}

func TestDocumentFilenameFallback(t *testing.T) {
	doc := &tg.Document{ID: 123}
	if got := documentFilename(doc); got != "document_123" {
		t.Errorf("fallback = %q", got)
	}
}

func TestResolveOutPath(t *testing.T) {
	if got := resolveOutPath("", "a.bin"); got != "a.bin" {
		t.Errorf("default = %q", got)
	}
	if got := resolveOutPath("file.out", "a.bin"); got != "file.out" {
		t.Errorf("explicit file = %q", got)
	}
	dir := t.TempDir()
	if got := resolveOutPath(dir, "a.bin"); got != filepath.Join(dir, "a.bin") {
		t.Errorf("dir join = %q", got)
	}
}

func TestValidateRemoteFilename(t *testing.T) {
	for _, name := range []string{
		"../escape", "..", ".", "/absolute", `C:\absolute`, `C:/absolute`, `C:relative`,
		`dir\file`, "dir/file", "bad:name", "name.", "name ", "CON", "con.txt", "NUL.log",
		"CON .txt", "CONIN$", "CONOUT$", "COM1", "COM¹", "LPT9.txt", "LPT².log",
		"bad\x1bname", "bad\nname", "bad\x00name",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRemoteFilename(name); err == nil {
				t.Fatalf("validateRemoteFilename(%q) succeeded", name)
			}
		})
	}
	if err := validateRemoteFilename("résumé.pdf"); err != nil {
		t.Fatalf("valid Unicode filename rejected: %v", err)
	}
	for _, name := range []string{"COM0", "COM10", "LPT0.txt", "report final.pdf"} {
		if err := validateRemoteFilename(name); err != nil {
			t.Errorf("valid filename %q rejected: %v", name, err)
		}
	}
}

func TestHasTrailingPathSeparator(t *testing.T) {
	for _, tc := range []struct {
		path      string
		separator rune
		want      bool
	}{
		{path: "folder/", separator: '/', want: true},
		{path: `folder\`, separator: '\\', want: true},
		{path: "folder/", separator: '\\', want: true},
		{path: "folder", separator: '\\', want: false},
	} {
		if got := hasTrailingPathSeparator(tc.path, tc.separator); got != tc.want {
			t.Errorf("hasTrailingPathSeparator(%q, %q) = %v, want %v", tc.path, tc.separator, got, tc.want)
		}
	}
}

func TestSaveDownloadCreatesExclusivelyAndStreams(t *testing.T) {
	dir := t.TempDir()
	result, err := saveDownload(dir, "résumé.bin", func(dst io.WriterAt) error {
		_, err := dst.WriteAt([]byte("media bytes"), 0)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Join(dir, "résumé.bin") || result.Size != int64(len("media bytes")) {
		t.Fatalf("result = %+v", result)
	}
	if got, err := os.ReadFile(result.Path); err != nil || !bytes.Equal(got, []byte("media bytes")) {
		t.Fatalf("download = %q, %v", got, err)
	}

	if _, err := saveDownload(dir, "résumé.bin", func(io.WriterAt) error { return nil }); err == nil {
		t.Fatal("existing output was overwritten")
	}
	if got, err := os.ReadFile(result.Path); err != nil || !bytes.Equal(got, []byte("media bytes")) {
		t.Fatalf("existing output changed: %q, %v", got, err)
	}
}

func TestSaveDownloadDoesNotFollowOutputSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "chosen.bin")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := saveDownload(link, "remote.bin", func(dst io.WriterAt) error {
		_, err := dst.WriteAt([]byte("replacement"), 0)
		return err
	}); err == nil {
		t.Fatal("download followed or replaced an output symlink")
	}
	if got, err := os.ReadFile(outside); err != nil || !bytes.Equal(got, []byte("preserve")) {
		t.Fatalf("symlink target changed: %q, %v", got, err)
	}
}

func TestSaveDownloadDoesNotRemoveReplacedTargetOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.bin")
	replacement := []byte("replacement")
	_, err := saveDownload(target, "file.bin", func(dst io.WriterAt) error {
		if _, err := dst.WriteAt([]byte("partial"), 0); err != nil {
			return err
		}
		if err := os.Remove(target); err != nil {
			return err
		}
		if err := os.WriteFile(target, replacement, 0o600); err != nil {
			return err
		}
		return errors.New("transfer failed")
	})
	if err == nil || !strings.Contains(err.Error(), "transfer failed") {
		t.Fatalf("error = %v, want transfer failure", err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("replacement target removed or changed: %q, %v", got, err)
	}
}

func TestSaveDownloadReportsRetainedPartialOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "partial.bin")
	_, err := saveDownload(dir, "partial.bin", func(dst io.WriterAt) error {
		_, err := dst.WriteAt([]byte("partial"), 0)
		if err != nil {
			return err
		}
		return errors.New("transfer failed")
	})
	if err == nil || !strings.Contains(err.Error(), target) {
		t.Fatalf("error = %v, want retained path %q", err, target)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, []byte("partial")) {
		t.Fatalf("partial download = %q, %v", got, err)
	}
}

func TestSaveDownloadRejectsInvalidRemoteFilenameBeforeUsingOut(t *testing.T) {
	out := filepath.Join(t.TempDir(), "explicit.bin")
	if _, err := saveDownload(out, "", func(io.WriterAt) error { return nil }); err == nil {
		t.Fatal("empty remote filename should be rejected")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("invalid filename created explicit output: %v", err)
	}
}
