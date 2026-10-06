package lsfrt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// A journal record is a completed call or code step, keyed by label and
// checked by hash, so a resume replays it instead of running it again.
type record struct {
	Label string          `json:"label"`
	Kind  string          `json:"kind"` // call | step
	Hash  string          `json:"hash"`
	Ret   json.RawMessage `json:"ret"`
	Flags []string        `json:"flags,omitempty"`
	At    time.Time       `json:"at"`
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// journalKey names a label's journal files. The hash suffix keeps labels that
// slug alike ("a:b", "a_b") apart.
func journalKey(label string) string {
	sum := sha256.Sum256([]byte(label))
	slug := unsafeChars.ReplaceAllString(label, "_")
	if len(slug) > 100 { // file names stay under NAME_MAX; the hash keeps keys distinct
		slug = slug[:100]
	}
	return slug + "-" + hex.EncodeToString(sum[:4])
}

// callHash identifies a call's inputs: the prompt before any skill prefix,
// the schema, model and effort. Role and permissions are left out, so a
// permission change does not invalidate completed work.
func callHash(prompt string, o Opts) string {
	h := sha256.New()
	h.Write([]byte(prompt))
	h.Write([]byte{0})
	h.Write(o.Schema)
	h.Write([]byte{0})
	h.Write([]byte(o.Model + "/" + o.Effort))
	return hex.EncodeToString(h.Sum(nil))
}

// noteLabel warns when a label is reused with other inputs in one launch.
// The journal holds one record per label, so a reused label overwrites its
// record and a resume re-runs both calls: labels must be unique per call.
func (rt *Runtime) noteLabel(key, hash, label string) {
	rt.seenMu.Lock()
	prev, ok := rt.seen[key]
	rt.seen[key] = hash
	rt.seenMu.Unlock()
	if ok && prev != hash {
		rt.event("label-reused", map[string]any{"label": label})
		rt.Log(label + ": label reused with different inputs; the journal keeps only the latest")
	}
}

func (rt *Runtime) recordPath(key string) string {
	return filepath.Join(rt.cfg.StateDir, "journal", key+".json")
}

func (rt *Runtime) readRecord(key string) (*record, error) {
	b, err := os.ReadFile(rt.recordPath(key))
	if err != nil {
		return nil, err
	}
	var r record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if string(r.Ret) == "null" {
		r.Ret = nil
	}
	return &r, nil
}

func (rt *Runtime) writeRecord(key string, r record) error {
	r.At = time.Now().UTC()
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return writeFileAtomic(rt.recordPath(key), append(b, '\n'))
}

// replayCall returns a completed call's recorded return. A replay costs
// nothing and does not count against the caps.
func (rt *Runtime) replayCall(key, hash, label string) (CallResult, bool) {
	r, err := rt.readRecord(key)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return CallResult{}, false
	case err != nil:
		rt.Log(fmt.Sprintf("%s: journal record unreadable, call re-runs: %v", label, err))
		return CallResult{}, false
	case r.Kind != "call" || r.Hash != hash:
		rt.event("cache-stale", map[string]any{"label": label})
		return CallResult{}, false
	}
	rt.event("cache-hit", map[string]any{"label": label, "flags": r.Flags})
	rt.Log(label + ": replayed from the journal")
	return CallResult{Value: r.Ret, Class: string(classOK), Flags: r.Flags, Replayed: true}, true
}

// recordCall journals a call's non-null return. A null is not recorded, so a
// resume retries it. A failed write is logged: the call's result still stands.
func (rt *Runtime) recordCall(key, hash, label string, res CallResult) {
	if err := rt.writeRecord(key, record{Label: label, Kind: "call", Hash: hash, Ret: res.Value, Flags: res.Flags}); err != nil {
		rt.Log(fmt.Sprintf("%s: journal write failed, a resume will re-run the call: %v", label, err))
	}
}

// Step journals a code step. A record with the same label and input hash is
// replayed without calling fn. Otherwise an intent is written, fn runs, and
// the record (null values included) replaces the intent. interrupted is true
// when an intent with this hash was left by an earlier launch, so fn's own
// effect check can skip an effect already applied. If fn errors, the intent
// stays and the error is returned as is. Step is a call boundary; in a dry
// run, a step that does not replay stops without calling fn.
func (rt *Runtime) Step(ctx context.Context, label string, input []byte, fn func(ctx context.Context, interrupted bool) (json.RawMessage, error)) (json.RawMessage, error) {
	key := journalKey(label)
	sum := sha256.Sum256(input)
	hash := hex.EncodeToString(sum[:])
	rt.noteLabel(key, hash, label)
	if r, err := rt.readRecord(key); err == nil && r.Kind == "step" && r.Hash == hash {
		rt.event("step-replayed", map[string]any{"label": label})
		return r.Ret, nil
	}
	if err := rt.boundary(ctx); err != nil {
		return nil, err
	}
	if rt.cfg.DryRun {
		rt.Log("dry run: step " + label + " not run")
		return nil, rt.setStop(StopDryRun, label)
	}
	intent := filepath.Join(rt.cfg.StateDir, "journal", key+".intent")
	prior, _ := os.ReadFile(intent)
	interrupted := string(prior) == hash
	if interrupted {
		rt.event("step-interrupted-rerun", map[string]any{"label": label})
	}
	if err := writeFileAtomic(intent, []byte(hash)); err != nil {
		return nil, rt.setStop(StopRuntimeError, fmt.Sprintf("step %s: intent: %v", label, err))
	}
	t0 := time.Now()
	ret, err := fn(ctx, interrupted)
	if err == nil {
		ret, err = compactJSON(ret)
	}
	if err != nil {
		rt.event("step-failed", map[string]any{"label": label, "error": err.Error()})
		return nil, err
	}
	if err := rt.writeRecord(key, record{Label: label, Kind: "step", Hash: hash, Ret: ret}); err != nil {
		// The effect is done but unrecorded: the intent stays, so a resume
		// reruns fn with interrupted=true and its effect check.
		rt.Log(fmt.Sprintf("step %s: journal write failed: %v", label, err))
	} else {
		os.Remove(intent)
	}
	rt.event("step", map[string]any{"label": label, "secs": time.Since(t0).Seconds(), "interrupted": interrupted})
	return ret, nil
}

// compactJSON is a value's journal form: the live return and its replay are
// byte-identical. Stream output is already compact.
func compactJSON(v json.RawMessage) (json.RawMessage, error) {
	if t := bytes.TrimSpace(v); len(t) == 0 || string(t) == "null" { // nil, empty and null are one value
		return nil, nil
	}
	var b bytes.Buffer
	if err := json.Compact(&b, v); err != nil {
		return nil, fmt.Errorf("return is not JSON: %w", err)
	}
	return b.Bytes(), nil
}
