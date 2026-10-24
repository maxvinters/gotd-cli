// Package privatefile reads and writes private regular files without following
// a final-component symlink.
package privatefile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Read returns a regular file's contents, refusing symlinks, replacements
// between the path check and open, and files readable by other Unix users.
func Read(path string) ([]byte, error) {
	root, name, err := openParent(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if !privatePermissions(before.Mode()) {
		return nil, fmt.Errorf("%s has permissions accessible to other users", path)
	}

	f, err := root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, fmt.Errorf("%s changed while opening", path)
	}
	if !privatePermissions(after.Mode()) {
		return nil, fmt.Errorf("%s has permissions accessible to other users", path)
	}
	return io.ReadAll(f)
}

// Write atomically replaces a regular file with private data.
func Write(path string, data []byte) error {
	return write(path, data, false)
}

// Create atomically creates a private file and refuses to replace any existing
// path.
func Create(path string, data []byte) error {
	return write(path, data, true)
}

func write(path string, data []byte, exclusive bool) error {
	root, name, err := openParent(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	if !exclusive {
		if info, err := root.Lstat(name); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("%s is not a regular file", path)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	tempName, err := randomName()
	if err != nil {
		return err
	}
	f, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = f.Close()
		if removeTemp {
			_ = root.Remove(tempName)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if exclusive {
		if err := root.Link(tempName, name); err != nil {
			return err
		}
		if err := root.Remove(tempName); err != nil {
			return err
		}
		removeTemp = false
		return nil
	}
	if err := root.Rename(tempName, name); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func openParent(path string) (*os.Root, string, error) {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return nil, "", fmt.Errorf("invalid file path %q", path)
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}
	if !privateDirectory(root) {
		_ = root.Close()
		return nil, "", fmt.Errorf("%s is in a group/world-writable directory", path)
	}
	return root, name, nil
}

func randomName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".private-" + hex.EncodeToString(b[:]), nil
}

func privatePermissions(mode os.FileMode) bool {
	return runtime.GOOS == "windows" || mode.Perm()&0o077 == 0
}

func privateDirectory(root *os.Root) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	info, err := root.Stat(".")
	return err == nil && info.IsDir() && info.Mode().Perm()&0o022 == 0
}
