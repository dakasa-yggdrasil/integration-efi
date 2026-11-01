package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/confidential"
)

const (
	confidentialRuntimePath = "/run/efi-confidential/runtime.json"
	confidentialRuntimeMax  = 4 * 1024
	printRuntimeArgument    = "--print-confidential-runtime"
)

var errPublicRuntime = errors.New("confidential public runtime unavailable")

// dispatchAdapterCommand runs the fixed public reader before the normal startup
// callback can initialize logging, credentials, SDK transport or private keys.
// The callbacks are private CI seams, never command-line or environment options.
func dispatchAdapterCommand(args []string, out, diagnostic io.Writer, start func(), read func() (confidential.RuntimePublic, error)) int {
	if len(args) == 0 {
		start()
		return 0
	}
	if len(args) != 1 || args[0] != printRuntimeArgument {
		_, _ = io.WriteString(diagnostic, "unsupported adapter arguments\n")
		return 2
	}
	metadata, err := read()
	if err != nil {
		_, _ = io.WriteString(diagnostic, "confidential public runtime unavailable\n")
		return 1
	}
	raw, err := encodePublicRuntime(metadata)
	if err != nil {
		_, _ = io.WriteString(diagnostic, "confidential public runtime unavailable\n")
		return 1
	}
	line := append(raw, '\n')
	if n, err := out.Write(line); err != nil || n != len(line) {
		_, _ = io.WriteString(diagnostic, "confidential public runtime unavailable\n")
		return 1
	}
	return 0
}

func readFixedPublicRuntime() (confidential.RuntimePublic, error) {
	return readPublicRuntime(confidentialRuntimePath)
}

// readPublicRuntime opens the fixed public file without following the final
// symlink or blocking on a FIFO. Validation uses this descriptor throughout.
func readPublicRuntime(path string) (confidential.RuntimePublic, error) {
	var empty confidential.RuntimePublic
	directoryFD, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return empty, errPublicRuntime
	}
	defer unix.Close(directoryFD)
	fd, err := unix.Openat(directoryFD, filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return empty, errPublicRuntime
	}
	file := os.NewFile(uintptr(fd), "confidential-public-runtime")
	if file == nil {
		_ = unix.Close(fd)
		return empty, errPublicRuntime
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > confidentialRuntimeMax {
		return empty, errPublicRuntime
	}
	raw, err := io.ReadAll(io.LimitReader(file, confidentialRuntimeMax+1))
	if err != nil || len(raw) == 0 || len(raw) > confidentialRuntimeMax {
		return empty, errPublicRuntime
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() || int64(len(raw)) != after.Size() {
		return empty, errPublicRuntime
	}
	metadata, err := confidential.DecodePublic(raw)
	if err != nil {
		return empty, errPublicRuntime
	}
	return metadata, nil
}

func publishFixedPublicRuntime(metadata confidential.RuntimePublic) error {
	return publishPublicRuntime(confidentialRuntimePath, metadata)
}

// publishPublicRuntime writes only the validated public DTO. A startup replaces
// stale public metadata in one rename; no private material is written here.
// The existing dedicated directory is supplied by the deployment's public tmpfs.
func publishPublicRuntime(path string, metadata confidential.RuntimePublic) error {
	raw, err := encodePublicRuntime(metadata)
	if err != nil {
		return errPublicRuntime
	}
	directory := filepath.Dir(path)
	directoryInfo, err := os.Lstat(directory)
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return errPublicRuntime
	}
	file, err := os.CreateTemp(directory, ".runtime-public-*")
	if err != nil {
		return errPublicRuntime
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return errPublicRuntime
	}
	if n, err := file.Write(raw); err != nil || n != len(raw) {
		return errPublicRuntime
	}
	if err := file.Sync(); err != nil {
		return errPublicRuntime
	}
	if err := file.Close(); err != nil {
		return errPublicRuntime
	}
	if err := os.Rename(name, path); err != nil {
		return errPublicRuntime
	}
	dir, err := os.Open(directory)
	if err != nil {
		return errPublicRuntime
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return errPublicRuntime
	}
	return nil
}

func encodePublicRuntime(metadata confidential.RuntimePublic) ([]byte, error) {
	raw, err := json.Marshal(metadata)
	if err != nil || len(raw) == 0 || len(raw) > confidentialRuntimeMax {
		return nil, errPublicRuntime
	}
	if _, err := confidential.DecodePublic(raw); err != nil {
		return nil, errPublicRuntime
	}
	return raw, nil
}
