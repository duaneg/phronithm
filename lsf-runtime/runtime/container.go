package lsfrt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Every agent call runs in its own rootless podman container
// (docs/design/lsf-runtime.md, Permissions → A call's launch). Its working
// directory, the call's clone, is bind-mounted at its host path, so the
// absolute paths in the clone's local config (core.hooksPath and the
// lsf-global files) resolve inside it as on the host; the clone's .git is
// bound over itself, so it cannot be renamed away, and its config, hooks
// and info are bound read-only over that. claude and the
// plugin are bound read-only; the network is pasta's, spliced to the
// credential proxy on the host's 127.0.0.1 and nothing else.

// In-container paths.
const (
	ctrClaude = "/opt/claude/claude"
	ctrPlugin = "/opt/plugin"
)

// Container labels: a launch's containers carry its state dir's label; the
// launch check's smoke container carries the smoke label too.
const (
	labelState = "lsf.state"
	labelSmoke = "lsf.smoke=1"
)

// gitROBinds are the clone's .git entries bound read-only over its
// writable bind.
var gitROBinds = []string{"config", "hooks", "info"}

// claudeArgv is a headless call's claude arguments. The prompt goes on
// stdin, so it is bounded by neither MAX_ARG_STRLEN nor a variadic flag
// swallowing it.
func claudeArgv(o Opts, role Role, sessionID string) []string {
	a := []string{"-p", "--output-format", "stream-json", "--verbose", "--session-id", sessionID}
	if o.Model != "" {
		a = append(a, "--model", o.Model)
	}
	if o.Effort != "" {
		a = append(a, "--effort", o.Effort)
	}
	return append(a, "--strict-mcp-config", "--plugin-dir", ctrPlugin, "--tools", role.Tools,
		"--dangerously-skip-permissions", "--json-schema", string(o.Schema))
}

// containerSpec is one container's inputs.
type containerSpec struct {
	name     string
	dir      string   // the working directory, bound at its host path
	gitBinds bool     // dir is a clone: bind its .git entries read-only
	network  string   // "" for pasta spliced to the proxy
	labels   []string // beyond the state label
	cmd      []string // after the image
}

// containerArgv is podman's arguments for a container. The placeholder
// bearer is not among them: -e ANTHROPIC_AUTH_TOKEN takes it from podman's
// environment (podmanEnv), as argv is visible to every local user. Each
// env passthrough name likewise takes its value from the runtime's own
// environment, which podman inherits and the container does not.
func (rt *Runtime) containerArgv(s containerSpec) []string {
	a := []string{"run", "--rm", "-i", "--init", "--pull=never", "--log-driver=none",
		"--name", s.name, "--label", labelState + "=" + rt.stateHash}
	for _, l := range s.labels {
		a = append(a, "--label", l)
	}
	network := s.network
	if network == "" {
		network = "pasta:--splice-only,-T," + rt.proxyPort()
	}
	a = append(a, "--userns=keep-id", "--cap-drop=all", "--security-opt", "no-new-privileges",
		"--read-only", "--tmpfs", "/tmp:rw,exec", "--network", network,
		"-v", s.dir+":"+s.dir+":rw")
	if s.gitBinds {
		// .git bound over itself is a mountpoint, which the agent cannot
		// rename or replace with a .git of its own.
		g := filepath.Join(s.dir, ".git")
		a = append(a, "-v", g+":"+g+":rw")
		for _, g := range gitROBinds {
			p := filepath.Join(s.dir, ".git", g)
			a = append(a, "-v", p+":"+p+":ro")
		}
	}
	a = append(a, "-v", rt.cfg.ClaudeBin+":"+ctrClaude+":ro", "-v", rt.cfg.PluginDir+":"+ctrPlugin+":ro",
		"-w", s.dir, "-e", "HOME=/tmp/home", "-e", "SHELL=/bin/bash", "-e", "DISABLE_AUTOUPDATER=1",
		"-e", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "-e", "ANTHROPIC_BASE_URL="+rt.Proxy().URL,
		"-e", "ANTHROPIC_AUTH_TOKEN", "-e", "NO_PROXY=127.0.0.1", "-e", "no_proxy=127.0.0.1")
	for _, e := range rt.cfg.Env {
		a = append(a, "-e", e)
	}
	a = append(a, rt.cfg.Image)
	return append(a, s.cmd...)
}

// rmTimeout bounds a container's removal, which runs whatever the call's
// context: a cancelled call's container must still go.
const rmTimeout = 60 * time.Second

// removeContainer removes a container, if it exists. -t 0: its processes
// have had SIGTERM and the grace, or were never running.
func (rt *Runtime) removeContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), rmTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, rt.cfg.PodmanBin, "rm", "-f", "-t", "0", "--ignore", name)
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("podman rm timed out after %v", rmTimeout)
		}
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	rt.event("container-removed", map[string]any{"container": name})
	return nil
}

// callContainer is a call attempt's container in placement p.
func (rt *Runtime) callContainer(c *callState, p placement, sessionID string) containerSpec {
	return containerSpec{
		name:     rt.containerName(sessionID),
		dir:      p.dir,
		gitBinds: p.git,
		cmd:      append([]string{ctrClaude}, claudeArgv(c.o, c.role, sessionID)...),
	}
}

// podmanEnv is podman's environment: the runtime's, with the placeholder
// bearer that -e ANTHROPIC_AUTH_TOKEN passes on.
func (rt *Runtime) podmanEnv() []string {
	return append(os.Environ(), "ANTHROPIC_AUTH_TOKEN="+rt.Proxy().Placeholder)
}

// proxyPort is the port the proxy is bound to.
func (rt *Runtime) proxyPort() string {
	if rt.proxy == nil {
		return "0"
	}
	_, port, _ := net.SplitHostPort(rt.proxy.ln.Addr().String())
	return port
}

// containerName is an attempt's container: unique per attempt, and naming
// its launch's state dir.
func (rt *Runtime) containerName(sessionID string) string {
	return "lsf-" + rt.stateHash[:8] + "-" + sessionID
}

// stateHashOf is the label value of a state dir's containers: the first 16
// hex digits of the SHA-256 of its resolved path, so every spelling of it
// gets the same label.
func stateHashOf(stateDir string) (string, error) {
	r, err := filepath.EvalSymlinks(stateDir)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(r))
	return hex.EncodeToString(sum[:])[:16], nil
}

// prepareGitBinds checks a clone's .git before its launch and creates its
// hooks and info directories when missing, so their read-only binds have a
// source.
func prepareGitBinds(dir string) error {
	git := filepath.Join(dir, ".git")
	if fi, err := os.Lstat(git); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory, so its config cannot be bound read-only", git)
	}
	if fi, err := os.Lstat(filepath.Join(git, "config")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s/config is not a regular file", git)
	}
	for _, d := range []string{"hooks", "info"} {
		if err := os.MkdirAll(filepath.Join(git, d), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// resolveBin is a binary's absolute path with its symlinks resolved, when it
// is a regular file: it is bind-mounted, and a symlink may move.
func resolveBin(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if p, err = filepath.Abs(p); err != nil {
		return "", err
	}
	if p, err = filepath.EvalSymlinks(p); err != nil {
		return "", err
	}
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", p)
	}
	if strings.Contains(p, ":") {
		return "", errors.New(p + ": a path with ':' cannot be a bind mount's source")
	}
	return p, nil
}
