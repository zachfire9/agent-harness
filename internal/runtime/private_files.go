package runtime

import (
	"os"
	"path/filepath"
)

// Instance runtime files (status, job state, job output, notification outbox,
// progress cursors, config) can contain generated email bodies, document
// excerpts, and account metadata, so they are readable by the owner only.
//
// On Unix these modes are enforced. On Windows Go only maps the write bit to
// the read-only attribute; privacy there comes from the per-user
// %LOCALAPPDATA% ACLs the instance home inherits.
const (
	privateDirMode  os.FileMode = 0o700
	privateFileMode os.FileMode = 0o600
)

// ensurePrivateDir creates dir (and parents) and tightens dir itself to
// owner-only, including when it already existed from an older release.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, privateDirMode); err != nil {
		return err
	}
	return os.Chmod(dir, privateDirMode)
}

// writePrivateFile writes data owner-only, tightening an existing file too:
// os.WriteFile only applies its mode when it creates the file.
func writePrivateFile(path string, data []byte) error {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, privateFileMode); err != nil {
		return err
	}
	return os.Chmod(path, privateFileMode)
}

// openPrivateAppend opens path for appending, creating it owner-only and
// tightening an existing file's mode.
func openPrivateAppend(path string) (*os.File, error) {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, privateFileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(privateFileMode); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
