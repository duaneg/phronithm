// Package testcfg is what the runtime's and the orchestrator's tests give a
// config so that its calls run through the stub podman (testdata/podman)
// with no podman installed.
package testcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Image is a well-formed digest naming no image; the stub accepts any.
const Image = "localhost/lsf-test@sha256:0000000000000000000000000000000000000000000000000000000000000000"

// ClaudeVersion is what the stub claude and the orchestrator's fake claude
// answer to --version.
const ClaudeVersion = "0.0.0-stub"

// Token is the live token in Container's credentials file.
const Token = "test-live-token"

// Testdata is the module's testdata directory.
func Testdata() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testcfg: no caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// Podman is the stub podman's path.
func Podman() string { return filepath.Join(Testdata(), "podman") }

// Claude is the stub claude's path.
func Claude() string { return filepath.Join(Testdata(), "claude") }

// Container is one test's container settings.
type Container struct {
	PluginDir   string // an empty directory
	Credentials string // a credentials file holding Token
	State       string // the stub podman's records (STUB_PODMAN_STATE)
}

// New makes a test's plugin dir and credentials file and points the stub
// podman's records at a fresh directory.
func New(t testing.TB) Container {
	t.Helper()
	c := Container{PluginDir: t.TempDir(), State: t.TempDir()}
	c.Credentials = filepath.Join(t.TempDir(), ".credentials.json")
	b, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{"accessToken": Token}})
	if err := os.WriteFile(c.Credentials, b, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_PODMAN_STATE", c.State)
	return c
}
