package orchestrator

import (
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// withContainer gives a test's config the stub podman's container settings
// and, when it names none, the stub claude.
func withContainer(t *testing.T, c lsfrt.Config) lsfrt.Config {
	t.Helper()
	tc := testcfg.New(t)
	c.PodmanBin, c.Image, c.ClaudeVersion, c.PluginDir = testcfg.Podman(), testcfg.Image, testcfg.ClaudeVersion, tc.PluginDir
	c.Proxy = &lsfrt.ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: tc.Credentials}
	if c.ClaudeBin == "" {
		c.ClaudeBin = testcfg.Claude()
	}
	return c
}
