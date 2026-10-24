package privatefile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteIsAtomicPrivateAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %04o, want 0600", got)
	}

	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Write(link, []byte("bad")); err == nil {
		t.Fatal("Write accepted symlink destination")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatalf("symlink target = %q, %v; want unchanged", got, err)
	}
}

func TestCreateDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := Create(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := Create(path, []byte("second")); err == nil {
		t.Fatal("Create overwrote an existing file")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "first" {
		t.Fatalf("file = %q, %v; want first content", got, err)
	}
}

func TestReadRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Read(link); err == nil {
		t.Fatal("Read followed symlink")
	}
}

func TestReadRejectsGroupOrOtherReadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("Read accepted a file readable by other users")
	}
	if err := Write(path, []byte("private")); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || string(got) != "private" {
		t.Fatalf("Read = %q, %v; want private contents", got, err)
	}
}

func TestRejectsGroupOrWorldWritableParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permission bits are not meaningful on Windows")
	}
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "group writable", mode: 0o775},
		{name: "world writable", mode: 0o777},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "shared")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "secret")
			if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(path); err == nil {
				t.Fatal("Read accepted writable parent directory")
			}
			if err := Write(path, []byte("replacement")); err == nil {
				t.Fatal("Write accepted writable parent directory")
			}
			if err := Create(filepath.Join(dir, "new"), []byte("new")); err == nil {
				t.Fatal("Create accepted writable parent directory")
			}
		})
	}
}

func TestReadAllowsReadableNonWritableParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permission bits are not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "public")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || string(got) != "secret" {
		t.Fatalf("Read = %q, %v; want secret", got, err)
	}
}
