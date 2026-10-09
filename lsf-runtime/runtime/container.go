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

	"github.com/duaneg/phronithm/lsf-runtime/internal/gitenv"
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
		"-e", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	if s.network == "" { // the API's route: only a call spliced to the proxy gets it
		a = append(a, "-e", "ANTHROPIC_BASE_URL="+rt.Proxy().URL, "-e", "ANTHROPIC_AUTH_TOKEN")
	}
	a = append(a, "-e", "NO_PROXY=127.0.0.1", "-e", "no_proxy=127.0.0.1")
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
	if err != nil && ctx.Err() == nil {
		// --rm's own removal may have been in flight: gone is gone.
		exists := exec.CommandContext(ctx, rt.cfg.PodmanBin, "container", "exists", name)
		if exitStatus(exists.Run()) == 1 {
			err = nil
		}
	}
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

// smokeTimeout bounds the launch check's smoke container.
const smokeTimeout = 2 * time.Minute

// smokeScript is the smoke container's check, run by /bin/sh in a
// throwaway repo bound as a call's clone is: claude's version on a marked
// line (podman may write warnings first), then
// the tools a call relies on, a write to the index, and the read-only .git
// binds and the .git mountpoint holding.
const smokeScript = `set -e
echo "lsf-smoke-claude $(/opt/claude/claude --version)"
bash -c true
git --version >/dev/null
rg --version >/dev/null
echo smoke > smoke.txt
git add smoke.txt
for p in .git/config .git/hooks/x .git/info/x; do
  if (echo x >> "$p") 2>/dev/null; then echo "$p is writable" >&2; exit 1; fi
done
if mv .git .git.moved 2>/dev/null; then echo ".git can be renamed" >&2; exit 1; fi
`

// checkContainerHost is the launch check (docs/design/lsf-runtime.md,
// Permissions → Image): podman present and rootless, pasta present, the
// image present, and a smoke container that reports the pinned claude and
// runs bash, git and rg with a call's mounts. A dry run runs it too.
func (rt *Runtime) checkContainerHost(ctx context.Context) error {
	if _, err := exec.LookPath(rt.cfg.PodmanBin); err != nil {
		return fmt.Errorf("podman: %w", err)
	}
	qctx, cancel := context.WithTimeout(ctx, rmTimeout)
	defer cancel()
	info := func(format string) (string, error) {
		out, err := exec.CommandContext(qctx, rt.cfg.PodmanBin, "info", "--format", format).Output()
		if err != nil {
			return "", fmt.Errorf("podman info: %w%s", err, stderrOf(err))
		}
		return strings.TrimSpace(string(out)), nil
	}
	rootless, err := info("{{.Host.Security.Rootless}}")
	if err != nil {
		return err
	}
	if rootless != "true" {
		return fmt.Errorf("podman is not rootless (Host.Security.Rootless %q)", rootless)
	}
	pasta, err := info("{{.Host.Pasta.Executable}}")
	if err != nil {
		return err
	}
	if pasta == "" {
		return errors.New("pasta is missing (podman info names no Host.Pasta.Executable)")
	}
	if fi, err := os.Stat(pasta); err != nil || fi.IsDir() {
		return fmt.Errorf("pasta: %s is not an executable on this host", pasta)
	}
	if out, err := exec.CommandContext(qctx, rt.cfg.PodmanBin, "image", "exists", rt.cfg.Image).CombinedOutput(); err != nil {
		return fmt.Errorf("image %s is not present (podman image exists: %v %s); build it and name it by digest", rt.cfg.Image, err, strings.TrimSpace(string(out)))
	}
	return rt.smoke(ctx)
}

func stderrOf(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return ": " + strings.TrimSpace(string(ee.Stderr))
	}
	return ""
}

// smoke runs the smoke container in <stateDir>/smoke, removed afterwards.
func (rt *Runtime) smoke(parent context.Context) error {
	dir := filepath.Join(rt.cfg.StateDir, "smoke")
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("smoke: %w", err)
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(parent, smokeTimeout)
	defer cancel()
	init := exec.CommandContext(ctx, "git", "init", "-q", dir)
	init.Env = gitenv.Env(true)
	if out, err := init.CombinedOutput(); err != nil {
		return fmt.Errorf("smoke: git init: %v %s", err, strings.TrimSpace(string(out)))
	}
	if err := prepareGitBinds(dir); err != nil {
		return fmt.Errorf("smoke: %w", err)
	}
	name := "lsf-" + rt.stateHash[:8] + "-smoke-" + rt.launchID
	cmd := exec.CommandContext(ctx, rt.cfg.PodmanBin, rt.containerArgv(containerSpec{
		name: name, dir: dir, gitBinds: true, network: "none", labels: []string{labelSmoke},
		cmd: []string{"/bin/sh", "-c", smokeScript},
	})...)
	cmd.Env = rt.podmanEnv()
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr tailBuffer
	stdout.max, stderr.max = 4096, 2048
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		if rerr := rt.removeContainer(name); rerr != nil {
			rt.Log("smoke: " + rerr.Error())
		}
		return fmt.Errorf("smoke container did not finish (%v): %s %s", context.Cause(ctx), stdout.String(), stderr.String())
	}
	if err != nil {
		return fmt.Errorf("smoke container failed (%v): %s %s", err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	var version string
	for _, l := range strings.Split(stdout.String(), "\n") {
		if v, ok := strings.CutPrefix(l, "lsf-smoke-claude "); ok {
			version = strings.TrimSpace(v)
		}
	}
	if f := strings.Fields(version); len(f) == 0 || f[0] != rt.cfg.ClaudeVersion {
		return fmt.Errorf("smoke container: claude --version reported %q, not claudeVersion %s", version, rt.cfg.ClaudeVersion)
	}
	rt.event("smoke", map[string]any{"claude": version})
	return nil
}
