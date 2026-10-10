package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// defaults are the plugin's label-bound settings: the roles, role rules
// and skill rules its stage labels need. A host file's key replaces the
// default's whole.
//
//go:embed defaults.json
var defaults []byte

// cliKeys are set per launch by the CLI, never by the host file.
var cliKeys = []string{"stateDir", "repo", "args", "dryRun"}

// oldKeys mark a config from before the container boundary.
var oldKeys = []string{"allow", "deny", "permissionMode"}

const configDoc = "docs/design/lsf-runtime.md, CLI → Configuration"

// hostConfigPath is the host file's default location.
func hostConfigPath() string {
	dir, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "$XDG_CONFIG_HOME/lsf-runtime/config.json (" + err.Error() + ")"
	}
	return filepath.Join(dir, "lsf-runtime", "config.json")
}

// launchIn is what a launch's config is composed from.
type launchIn struct {
	hostFile  string // "" for hostConfigPath
	pluginDir string // -plugin-dir, "" when not given
	repo      string
	stateDir  string
	args      json.RawMessage
	dryRun    bool
	out       string // the composed file
}

// compose writes the launch's config to in.out and loads it: the plugin
// defaults, the host file over them key by key, then the CLI's fields and
// claudeBin resolved once. A load error leaves no file.
func compose(in launchIn) (lsfrt.Config, error) {
	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(defaults, &merged); err != nil {
		return lsfrt.Config{}, fmt.Errorf("embedded defaults: %w", err)
	}
	host, hostPath, err := readHost(in.hostFile)
	if err != nil {
		return lsfrt.Config{}, err
	}
	maps.Copy(merged, host)
	hostDir := filepath.Dir(hostPath)
	var hostStr = map[string]string{}
	for _, k := range []string{"pluginDir", "claudeBin", "claudeVersion"} {
		if v, ok := host[k]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return lsfrt.Config{}, fmt.Errorf("host config %s: %s must be a string, not %s", hostPath, k, v)
			}
			hostStr[k] = s
		}
	}
	if s := hostStr["pluginDir"]; s != "" && !filepath.IsAbs(s) {
		merged["pluginDir"] = jsonString(filepath.Join(hostDir, s))
	}
	if in.pluginDir != "" {
		p, err := filepath.Abs(in.pluginDir)
		if err != nil {
			return lsfrt.Config{}, err
		}
		merged["pluginDir"] = jsonString(p)
	}
	src, from := os.Getenv("LSF_CLAUDE_BIN"), "LSF_CLAUDE_BIN"
	if s := hostStr["claudeBin"]; src == "" && s != "" {
		src, from = s, "the host config's claudeBin"
		if strings.Contains(s, "/") && !filepath.IsAbs(s) {
			src = filepath.Join(hostDir, s)
		}
	}
	if src == "" {
		src, from = "claude", "the default"
	}
	bin, version, err := resolveClaude(src)
	if err != nil {
		return lsfrt.Config{}, fmt.Errorf("%w (claudeBin %s, from %s)", err, src, from)
	}
	if want := hostStr["claudeVersion"]; want != "" && want != version {
		return lsfrt.Config{}, fmt.Errorf("claudeBin %s reports version %s; the host config's claudeVersion is %s", bin, version, want)
	}
	merged["claudeBin"], merged["claudeVersion"] = jsonString(bin), jsonString(version)
	merged["repo"], merged["stateDir"], merged["args"] = jsonString(in.repo), jsonString(in.stateDir), in.args
	if in.dryRun {
		merged["dryRun"] = json.RawMessage("true")
	}
	b, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return lsfrt.Config{}, err
	}
	if err := os.WriteFile(in.out, append(b, '\n'), 0o600); err != nil {
		return lsfrt.Config{}, err
	}
	cfg, err := lsfrt.LoadConfig(in.out)
	if err != nil {
		os.Remove(in.out)
		// The composed file is gone; the fault is in the host file or a flag.
		msg := strings.Replace(err.Error(), "config "+in.out+": ", "", 1)
		return lsfrt.Config{}, oldConfigHint(fmt.Errorf("config composed from %s: %s", hostPath, msg))
	}
	// LoadConfig applied LSF_CLAUDE_BIN over the file; the launch uses the
	// path resolved here.
	cfg.ClaudeBin = bin
	return cfg, nil
}

// readHost reads the host file, a JSON object without the CLI's keys.
func readHost(path string) (map[string]json.RawMessage, string, error) {
	if path == "" {
		path = hostConfigPath()
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("host config %s does not exist: create it (lsf-runtime config init; %s) or pass -config", path, configDoc)
	}
	if err != nil {
		return nil, "", err
	}
	var host map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&host); err != nil {
		return nil, "", fmt.Errorf("host config %s: not a JSON object: %v", path, err)
	}
	if host == nil {
		return nil, "", fmt.Errorf("host config %s: not a JSON object", path)
	}
	if dec.More() {
		return nil, "", fmt.Errorf("host config %s: trailing data after the config object", path)
	}
	var set []string
	for _, k := range cliKeys {
		if _, ok := host[k]; ok {
			set = append(set, k)
		}
	}
	if len(set) > 0 {
		return nil, "", fmt.Errorf("host config %s: set by the CLI per launch, remove: %s", path, strings.Join(set, ", "))
	}
	if u := unfilled(host); len(u) > 0 {
		return nil, "", fmt.Errorf("host config %s: fill in the starter's %s fields: %s", path, required, strings.Join(u, ", "))
	}
	return host, path, nil
}

func stringKey(m map[string]json.RawMessage, k string) (string, bool) {
	var s string
	if v, ok := m[k]; !ok || json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// oldConfigHint adds the remedy to a load error naming a key from before
// the container boundary.
func oldConfigHint(err error) error {
	for _, k := range oldKeys {
		if strings.Contains(err.Error(), fmt.Sprintf("unknown field %q", k)) {
			return fmt.Errorf("%w\nthis config predates the container boundary (docs/design/lsf-runtime.md, Permissions); remove the key", err)
		}
	}
	return err
}

// claudeVersionTimeout bounds claudeBin --version.
const claudeVersionTimeout = 30 * time.Second

// resolveClaude resolves a claude binary to its versioned path, a regular
// file with its symlinks resolved, and reads its version, the first field
// of its --version output.
func resolveClaude(name string) (path, version string, err error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", "", fmt.Errorf("claudeBin: %w", err)
	}
	if p, err = filepath.Abs(p); err != nil {
		return "", "", fmt.Errorf("claudeBin: %w", err)
	}
	if p, err = filepath.EvalSymlinks(p); err != nil {
		return "", "", fmt.Errorf("claudeBin: %w", err)
	}
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
		return "", "", fmt.Errorf("claudeBin: %s is not a regular file", p)
	}
	if strings.Contains(p, ":") {
		return "", "", fmt.Errorf("claudeBin: %s: a path with ':' cannot be a bind mount's source", p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, "--version")
	cmd.Env = append(os.Environ(), "DISABLE_AUTOUPDATER=1")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	f := strings.Fields(string(out))
	if err != nil || len(f) == 0 {
		return "", "", fmt.Errorf("claudeBin: %s --version: %q %v", p, strings.TrimSpace(string(out)), err)
	}
	return p, f[0], nil
}

// minGit is the oldest git the runtime runs on: git merge-tree
// --write-tree.
var minGit = [2]int{2, 38}

var gitVersionRe = regexp.MustCompile(`^git version (\d+)\.(\d+)`)

// checkGit refuses a git older than minGit, naming the version found.
func checkGit() error {
	out, err := exec.Command("git", "version").Output()
	if err != nil {
		return fmt.Errorf("git version: %v", err)
	}
	v := strings.TrimSpace(string(out))
	m := gitVersionRe.FindStringSubmatch(v)
	if m == nil {
		return fmt.Errorf("git version: cannot parse %q; git %d.%d or later is required (git merge-tree --write-tree)", v, minGit[0], minGit[1])
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < minGit[0] || major == minGit[0] && minor < minGit[1] {
		return fmt.Errorf("%s is older than %d.%d (git merge-tree --write-tree)", strings.TrimPrefix(v, "git version "), minGit[0], minGit[1])
	}
	return nil
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// checkName applies the name rule: nameRe, and a valid pause ref.
func checkName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("name %q: want letters, digits, '.', '_' or '-', starting with a letter or digit", name)
	}
	if err := exec.Command("git", "check-ref-format", "refs/lsf/"+name+"/pause").Run(); err != nil {
		return fmt.Errorf("name %q: refs/lsf/%s/pause is not a valid ref name", name, name)
	}
	return nil
}

// argsName reads args' name and checks it, and that it matches flagName
// when that is set.
func argsName(args json.RawMessage, flagName string) (string, error) {
	var a map[string]json.RawMessage
	if err := json.Unmarshal(args, &a); err != nil || a == nil {
		return "", fmt.Errorf("args: not a JSON object: %v", err)
	}
	name, ok := stringKey(a, "name")
	if !ok {
		return "", errors.New("args: name is required, a string")
	}
	if err := checkName(name); err != nil {
		return "", fmt.Errorf("args: %w", err)
	}
	if flagName != "" && flagName != name {
		return "", fmt.Errorf("-name %s differs from args.name %s", flagName, name)
	}
	return name, nil
}

// sortedKeys is m's keys in order, for messages.
func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }
