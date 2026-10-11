package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func init() {
	commands["config"] = command{run: cmdConfig, usage: "config init [-config FILE] [-plugin-dir DIR]"}
}

// required marks a starter host config's value the requester must supply;
// compose refuses a host file holding a string starting with it.
const required = "REQUIRED:"

func cmdConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "init" {
		fmt.Fprintln(stderr, "usage: lsf-runtime config init [-config FILE] [-plugin-dir DIR]")
		return exitUsage
	}
	fs := newFlags("config init", stderr)
	file := fs.String("config", "", "the host config to write (default: "+hostConfigPath()+")")
	pluginDir := fs.String("plugin-dir", "", "the plugin's root, named in the image instruction")
	if err := parse(fs, args[1:]); err != nil {
		return fail(stderr, err)
	}
	path := *file
	if path == "" {
		dir, err := xdgDir("XDG_CONFIG_HOME", ".config")
		if err != nil {
			return fail(stderr, fmt.Errorf("no default host config path: %w; pass -config", err))
		}
		path = filepath.Join(dir, "lsf-runtime", "config.json")
	}
	if err := writeStarter(path, *pluginDir); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, path)
	return exitOK
}

// writeStarter writes the starter host config to path, never over an
// existing file: the image and, without ~/.claude/.credentials.json, the
// proxy's credentials file are marked required.
func writeStarter(path, pluginDir string) error {
	containerfile := "<plugin>/lsf-runtime/container/Containerfile"
	if pluginDir != "" {
		p, err := filepath.Abs(pluginDir)
		if err != nil {
			return err
		}
		containerfile = filepath.Join(p, "lsf-runtime", "container", "Containerfile")
	}
	creds := required + " the claude credentials file (~/.claude/.credentials.json)"
	if home, err := os.UserHomeDir(); err == nil {
		if p := filepath.Join(home, ".claude", ".credentials.json"); fileExists(p) {
			creds = p
		}
	}
	starter := map[string]any{
		"image": required + " build an image from " + containerfile + " with the project's toolchain and put name@sha256:<digest> here (" + configDoc + ")",
		"proxy": map[string]any{"listen": "127.0.0.1:0", "credentialsFile": creds},
		"env":   []string{},
	}
	b, err := json.MarshalIndent(starter, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("host config %s exists; not overwritten", path)
	}
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// unfilled are the host file's string values still marked required, as
// dotted key paths.
func unfilled(host map[string]json.RawMessage) []string {
	var out []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch v := v.(type) {
		case string:
			if strings.HasPrefix(v, required) {
				out = append(out, prefix)
			}
		case map[string]any:
			for _, k := range sortedKeys(v) {
				walk(prefix+"."+k, v[k])
			}
		case []any:
			for i, e := range v {
				walk(fmt.Sprintf("%s[%d]", prefix, i), e)
			}
		}
	}
	for _, k := range sortedKeys(host) {
		var v any
		if json.Unmarshal(host[k], &v) == nil {
			walk(k, v)
		}
	}
	slices.Sort(out)
	return out
}
