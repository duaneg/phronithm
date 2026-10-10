package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// config init writes the starter 0600 in a 0700 directory, with the image
// marked required, and never overwrites.
func TestConfigInit(t *testing.T) {
	f := newFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "new", "config.json")
	r := f.cli(nil, "config", "init", "-config", path, "-plugin-dir", "/p")
	eq(t, r.code, exitOK, r.stderr)
	eq(t, r.stdout, path+"\n")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, fi.Mode().Perm(), os.FileMode(0o600))
	di, _ := os.Stat(filepath.Dir(path))
	eq(t, di.Mode().Perm(), os.FileMode(0o700))
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	img, _ := got["image"].(string)
	if !strings.HasPrefix(img, required) || !strings.Contains(img, "/p/lsf-runtime/container/Containerfile") {
		t.Errorf("image %q", img)
	}
	creds, _ := got["proxy"].(map[string]any)["credentialsFile"].(string)
	if !strings.HasPrefix(creds, required) {
		t.Errorf("credentialsFile %q without ~/.claude/.credentials.json", creds)
	}
	if _, ok := got["caps"]; ok {
		t.Error("starter has caps")
	}

	before := readFile(t, path)
	r = f.cli(nil, "config", "init", "-config", path)
	eq(t, r.code, exitError)
	contains(t, r.stderr, path+" exists; not overwritten")
	eq(t, readFile(t, path), before, "the file is unchanged")

	// With the credentials file present, the starter names it.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte("{}"), 0o600)
	path2 := filepath.Join(t.TempDir(), "config.json")
	eq(t, f.cli(nil, "config", "init", "-config", path2).code, exitOK)
	contains(t, readFile(t, path2), `"credentialsFile": "`+filepath.Join(home, ".claude", ".credentials.json")+`"`)

	eq(t, f.cli(nil, "config").code, exitUsage)
	eq(t, f.cli(nil, "config", "other").code, exitUsage)
}

// A launch over an unfilled starter is refused, naming its fields.
func TestLaunchRefusesStarter(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HOME", t.TempDir())
	os.Remove(f.host)
	if r := f.cli(nil, "config", "init", "-config", f.host); r.code != exitOK {
		t.Fatal(r.stderr)
	}
	r := f.launch(nil)
	eq(t, r.code, exitError)
	contains(t, r.stderr, "host config "+f.host+": fill in the starter's REQUIRED: fields: image, proxy.credentialsFile")
}

func TestVersion(t *testing.T) {
	f := newFixture(t)
	r := f.cli(nil, "version")
	eq(t, r.code, exitOK)
	eq(t, r.stdout, "dev unknown\n")
}

// Status lists the state dir's retain staging and scratch directories in
// the stores its records name, under the prefix of launch.json's
// stateDir, and not another state dir's.
func TestStatusRetainLeftovers(t *testing.T) {
	f := newFixture(t)
	st := f.stateDir()
	store, other := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(st, "retained"), 0o700)
	rec := func(name, store string) {
		b, _ := json.Marshal(M{"phase": "done", "unit": name, "store": store})
		os.WriteFile(filepath.Join(st, "retained", name+".json"), b, 0o600)
	}
	rec("u", store)
	rec("v", store)
	rec("w", other)
	rec("x", store+"/")
	rec("y", filepath.Join(t.TempDir(), "absent"))
	// launch.json spells the state dir differently from the CLI's
	// address: through a symlink, which the prefix's hash does not resolve.
	spelled := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(st, spelled); err != nil {
		t.Fatal(err)
	}
	if lsfrt.RetainPrefix(spelled) == lsfrt.RetainPrefix(st) {
		t.Fatal("the spellings hash alike")
	}
	b, _ := json.Marshal(M{"repo": f.repo, "stateDir": spelled, "args": M{"name": "f"}})
	os.WriteFile(filepath.Join(st, launchFile), b, 0o600)
	mine := lsfrt.RetainPrefix(spelled)
	for _, d := range []string{
		filepath.Join(store, mine+"u.partial"),
		filepath.Join(store, mine+"gone.copy"),
		filepath.Join(store, lsfrt.RetainPrefix("/elsewhere")+"u.partial"),
		filepath.Join(store, "data"),
		filepath.Join(other, mine+"w.partial"),
	} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(st, "retained", "bad.json"), []byte("{"), 0o600)
	s, _ := f.status()
	eq(t, strings.Join(s.Leftovers.Retain, ","), strings.Join([]string{
		filepath.Join(store, mine+"gone.copy"),
		filepath.Join(store, mine+"u.partial"),
		filepath.Join(other, mine+"w.partial"),
	}, ","))
	if len(s.Errors) != 1 || !strings.Contains(s.Errors[0], "bad.json") {
		t.Errorf("errors %q", s.Errors)
	}
}
