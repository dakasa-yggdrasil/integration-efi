package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/confidential"
)

func testRuntimeMetadata(t *testing.T) confidential.RuntimePublic {
	t.Helper()
	// X25519's public base point is a valid public fixture. No receiver,
	// private key or credential initialization is needed to exercise the reader.
	public := make([]byte, 32)
	public[0] = 9
	metadata := confidential.RuntimePublic{
		SchemaVersion:  1,
		Algorithm:      confidential.Algorithm,
		AdapterVersion: "2.5.7",
		KeyID:          "3c8719cf-4e80-4ecb-9b3e-2d14c3acc73c",
		PublicKey:      base64.StdEncoding.EncodeToString(public),
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := confidential.DecodePublic(raw); err != nil {
		t.Fatalf("invalid public-only fixture: %v", err)
	}
	return metadata
}

func TestFixedPublicReaderBypassesAllNormalStartup(t *testing.T) {
	metadata := testRuntimeMetadata(t)
	var output, diagnostic bytes.Buffer
	started, reads := 0, 0
	code := dispatchAdapterCommand([]string{printRuntimeArgument}, &output, &diagnostic, func() {
		// All logging, SDK, credential and receiver initialization is inside
		// this callback in main, and must be unreachable in the child mode.
		started++
	}, func() (confidential.RuntimePublic, error) {
		reads++
		return metadata, nil
	})
	if code != 0 || started != 0 || reads != 1 || diagnostic.Len() != 0 {
		t.Fatalf("reader dispatch: code=%d startup=%d reads=%d diagnostic=%q", code, started, reads, diagnostic.String())
	}
	decoded, err := confidential.DecodePublic(output.Bytes())
	if err != nil {
		t.Fatalf("reader output is not the closed public DTO: %v", err)
	}
	got, _ := json.Marshal(decoded)
	want, _ := json.Marshal(metadata)
	if !bytes.Equal(got, want) || !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
		t.Fatal("reader output changed the public DTO")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil || len(fields) != 5 {
		t.Fatal("reader emitted fields outside the closed public DTO")
	}
	for _, name := range []string{"schemaVersion", "algorithm", "adapterVersion", "keyID", "publicKey"} {
		if _, exists := fields[name]; !exists {
			t.Fatal("reader public DTO fields changed")
		}
	}
}

func TestPublicReaderRejectsArgumentExtensionsBeforeStartupOrRead(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown"},
		{printRuntimeArgument, "--path", "/private/secret"},
		{printRuntimeArgument + "=/private/secret"},
		{"--path", printRuntimeArgument},
		{printRuntimeArgument, printRuntimeArgument},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var output, diagnostic bytes.Buffer
			called := false
			code := dispatchAdapterCommand(args, &output, &diagnostic, func() { called = true }, func() (confidential.RuntimePublic, error) {
				called = true
				return confidential.RuntimePublic{}, nil
			})
			if code != 2 || called || output.Len() != 0 || diagnostic.String() != "unsupported adapter arguments\n" {
				t.Fatalf("unsupported arguments reached startup/read: code=%d called=%v output=%q diagnostic=%q", code, called, output.String(), diagnostic.String())
			}
		})
	}
}

func TestNormalAdapterStartupDoesNotReadPublicMetadata(t *testing.T) {
	started := 0
	code := dispatchAdapterCommand(nil, &bytes.Buffer{}, &bytes.Buffer{}, func() { started++ }, func() (confidential.RuntimePublic, error) {
		t.Fatal("normal startup invoked child reader")
		return confidential.RuntimePublic{}, nil
	})
	if code != 0 || started != 1 {
		t.Fatalf("normal startup: code=%d starts=%d", code, started)
	}
}

func TestPublicReaderFailureDoesNotReflectDiagnostics(t *testing.T) {
	var output, diagnostic bytes.Buffer
	code := dispatchAdapterCommand([]string{printRuntimeArgument}, &output, &diagnostic, func() { t.Fatal("reader failure started adapter") }, func() (confidential.RuntimePublic, error) {
		return confidential.RuntimePublic{}, errors.New("private provider key and credential contents")
	})
	if code != 1 || output.Len() != 0 || diagnostic.String() != "confidential public runtime unavailable\n" {
		t.Fatalf("unsafe reader failure: code=%d output=%q diagnostic=%q", code, output.String(), diagnostic.String())
	}
}

func TestPublicRuntimePublicationReplacesStaleFileAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	oldMetadata, newMetadata := testRuntimeMetadata(t), testRuntimeMetadata(t)
	newMetadata.KeyID = "fc296b8d-42db-419e-a6e7-52136555382a"
	if err := publishPublicRuntime(path, oldMetadata); err != nil {
		t.Fatalf("publish initial metadata: %v", err)
	}
	oldFile, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer oldFile.Close()
	oldInfo, err := oldFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := publishPublicRuntime(path, newMetadata); err != nil {
		t.Fatalf("replace stale metadata: %v", err)
	}
	newInfo, err := os.Stat(path)
	if err != nil || os.SameFile(oldInfo, newInfo) {
		t.Fatal("publication rewrote the old inode instead of atomic replacement")
	}
	if newInfo.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected public file permissions: %v", newInfo.Mode())
	}
	got, err := readPublicRuntime(path)
	if err != nil {
		t.Fatalf("read replaced public metadata: %v", err)
	}
	gotRaw, _ := json.Marshal(got)
	wantRaw, _ := json.Marshal(newMetadata)
	if !bytes.Equal(gotRaw, wantRaw) {
		t.Fatal("reader observed stale public metadata")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "runtime.json" {
		t.Fatal("publication left temporary files")
	}
}

func TestPublicRuntimeInvalidPublicationPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	metadata := testRuntimeMetadata(t)
	if err := publishPublicRuntime(path, metadata); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishPublicRuntime(path, confidential.RuntimePublic{}); err != errPublicRuntime {
		t.Fatalf("invalid public DTO was accepted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid publication damaged existing public metadata")
	}
	if err := publishPublicRuntime(filepath.Join(t.TempDir(), "missing", "runtime.json"), metadata); err != errPublicRuntime {
		t.Fatalf("missing dedicated directory was created or accepted: %v", err)
	}
}

func TestPublicRuntimeReaderRejectsUnsafeFiles(t *testing.T) {
	metadata := testRuntimeMetadata(t)
	valid, err := encodePublicRuntime(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "empty", "oversize", "directory", "symlink", "fifo", "unknown-field", "duplicate-field"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "runtime.json")
			switch kind {
			case "missing":
			case "empty":
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(path, bytes.Repeat([]byte(" "), confidentialRuntimeMax+1), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(dir, "target")
				if err := os.WriteFile(target, valid, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			case "unknown-field":
				raw := append([]byte(nil), valid[:len(valid)-1]...)
				raw = append(raw, []byte(",\"privateKey\":\"must-not-be-reflected\"}")...)
				if err := os.WriteFile(path, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			case "duplicate-field":
				raw := append([]byte(nil), valid[:len(valid)-1]...)
				raw = append(raw, []byte(",\"schemaVersion\":1}")...)
				if err := os.WriteFile(path, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readPublicRuntime(path); err != errPublicRuntime {
				t.Fatalf("unsafe file accepted or diagnostic reflected: %v", err)
			}
		})
	}
}

func TestPublicRuntimePublicationDoesNotFollowExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "runtime.json")
	if err := os.WriteFile(target, []byte("unchanged target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := publishPublicRuntime(path, testRuntimeMetadata(t)); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "unchanged target" {
		t.Fatal("publication followed a stale symlink")
	}
	if _, err := readPublicRuntime(path); err != nil {
		t.Fatalf("replacement is not a valid regular public file: %v", err)
	}
}

func TestPublicRuntimeRejectsSymlinkedDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := testRuntimeMetadata(t)
	if err := publishPublicRuntime(filepath.Join(target, "runtime.json"), metadata); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(link, "runtime.json")
	if _, err := readPublicRuntime(path); err != errPublicRuntime {
		t.Fatalf("reader followed symlinked public directory: %v", err)
	}
	if err := publishPublicRuntime(path, metadata); err != errPublicRuntime {
		t.Fatalf("publisher followed symlinked public directory: %v", err)
	}
}

func TestPublicRuntimeReaderAcceptsExactBoundWithoutTruncation(t *testing.T) {
	raw, err := encodePublicRuntime(testRuntimeMetadata(t))
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, bytes.Repeat([]byte(" "), confidentialRuntimeMax-len(raw))...)
	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPublicRuntime(path); err != nil {
		t.Fatalf("valid metadata at the exact public bound was rejected: %v", err)
	}
}

func TestReadinessRequiresInitializationWhileLivenessRemainsAvailable(t *testing.T) {
	readiness := &readinessState{}
	server := newHealthServer(readiness)
	for _, path := range []string{"/healthz", "/readyz"} {
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		want := http.StatusOK
		if path == "/readyz" {
			want = http.StatusServiceUnavailable
		}
		if response.Code != want {
			t.Fatalf("%s before initialization: %d", path, response.Code)
		}
	}
	readiness.ready.Store(true)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK || response.Body.String() != "ready" {
		t.Fatal("initialized runtime did not become ready")
	}
}
