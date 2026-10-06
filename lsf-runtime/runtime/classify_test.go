package lsfrt

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const verdictSchema = `{"type":"object","properties":{"verdict":{"type":"string","enum":["pass","fail"]}},"required":["verdict"]}`
const findingsSchema = `{"type":"object","properties":{"findings":{"type":"array","items":{"type":"string"}}},"required":["findings"]}`

func accFromLines(lines ...string) streamAcc {
	var a streamAcc
	for _, l := range lines {
		a.add([]byte(l))
	}
	return a
}

func accFromFixture(t *testing.T, name string) streamAcc {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "testdata", "streams", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var a streamAcc
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		a.add(sc.Bytes())
	}
	if sc.Err() != nil {
		t.Fatal(sc.Err())
	}
	return a
}

const (
	initLine     = `{"type":"system","subtype":"init","session_id":"s1","slash_commands":["phronithm:critique"]}`
	okResult     = `{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"pass"},"total_cost_usd":0.02}`
	limitAsst    = `{"type":"assistant","error":"rate_limit","message":{"content":[{"type":"text","text":"You've hit your session limit · resets 3:20am (Pacific/Auckland)"}]}}`
	limitResult  = `{"type":"result","subtype":"success","is_error":true,"api_error_status":429,"total_cost_usd":0}`
	limitEvent   = `{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1791252000,"rateLimitType":"five_hour"}}`
	allowedEvent = `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1791252000,"rateLimitType":"five_hour"}}`
)

func TestClassifyFixtures(t *testing.T) {
	for _, tc := range []struct {
		fixture, schema, skill string
		prefixed               bool
		want                   class
	}{
		{"ok", verdictSchema, "", false, classOK},
		{"permanent", verdictSchema, "", false, classPermanent},
		{"skill-loaded", findingsSchema, "phronithm:critique", true, classOK},
		{"skill-tool", findingsSchema, "phronithm:critique", false, classOK},
		{"skill-absent", findingsSchema, "phronithm:critique", false, classSkillMissing},
		{"skill-unknown", findingsSchema, "phronithm:nonexistent-skill", true, classSkillMissing},
		// A schema the output fails is invalid, whatever else the stream shows.
		{"ok", findingsSchema, "", false, classInvalid},
	} {
		a := &attempt{acc: accFromFixture(t, tc.fixture)}
		if got := classify(a, []byte(tc.schema), tc.skill, tc.prefixed).class; got != tc.want {
			t.Errorf("%s (skill %q, prefixed %v): class %s, want %s", tc.fixture, tc.skill, tc.prefixed, got, tc.want)
		}
	}
}

func TestClassifySynthetic(t *testing.T) {
	for _, tc := range []struct {
		name  string
		a     attempt
		want  class
		reset time.Time
	}{
		{"error then valid result is ok", attempt{acc: accFromLines(initLine,
			`{"type":"assistant","error":"overloaded","message":{"content":[]}}`, okResult)}, classOK, time.Time{}},
		{"529 is transient", attempt{exitCode: 1, acc: accFromLines(initLine,
			`{"type":"assistant","error":"overloaded","message":{"content":[]}}`,
			`{"type":"result","subtype":"success","is_error":true,"api_error_status":529}`)}, classTransient, time.Time{}},
		{"no result is transient", attempt{exitCode: 1, acc: accFromLines(initLine)}, classTransient, time.Time{}},
		{"nonzero exit without result is transient", attempt{exitCode: 2}, classTransient, time.Time{}},
		{"session limit with reset", attempt{exitCode: 1, acc: accFromLines(initLine, limitAsst, limitEvent, limitResult)},
			classSessionLimit, time.Unix(1791252000, 0)},
		{"session limit without a rejected event", attempt{exitCode: 1, acc: accFromLines(initLine, allowedEvent, limitAsst, limitResult)},
			classSessionLimit, time.Time{}},
		{"null structured_output is invalid", attempt{acc: accFromLines(initLine,
			`{"type":"result","subtype":"success","is_error":false,"structured_output":null}`)}, classInvalid, time.Time{}},
		{"missing structured_output is invalid", attempt{acc: accFromLines(initLine,
			`{"type":"result","subtype":"success","is_error":false}`)}, classInvalid, time.Time{}},
		{"401 is permanent", attempt{exitCode: 1, acc: accFromLines(
			`{"type":"result","subtype":"success","is_error":true,"api_error_status":401}`)}, classPermanent, time.Time{}},
		{"guard timeout before a result", attempt{cause: errGuard, acc: accFromLines(initLine)}, classTimeout, time.Time{}},
		{"guard timeout after a valid result keeps it", attempt{cause: errGuard, acc: accFromLines(initLine, okResult)}, classOK, time.Time{}},
		{"guard timeout after an invalid result", attempt{cause: errGuard, acc: accFromLines(initLine,
			`{"type":"result","subtype":"success","is_error":false,"structured_output":null}`)}, classTimeout, time.Time{}},
		{"repeated signal", attempt{cause: errKilled}, classStopped, time.Time{}},
		{"spawn error", attempt{spawnErr: errors.New("exec: not found")}, classSpawnError, time.Time{}},
		{"bad lines are counted, not fatal", attempt{acc: accFromLines("not json", initLine, okResult)}, classOK, time.Time{}},
	} {
		v := classify(&tc.a, []byte(verdictSchema), "", false)
		if v.class != tc.want || !v.resetsAt.Equal(tc.reset) {
			t.Errorf("%s: got %s (reset %v), want %s (reset %v)", tc.name, v.class, v.resetsAt, tc.want, tc.reset)
		}
	}
	if a := accFromLines("not json", initLine); a.BadLines != 1 || a.SessionID != "s1" {
		t.Errorf("bad line accounting: %+v", a)
	}
}

func TestClassifyOkCarriesOutputAndDenials(t *testing.T) {
	a := &attempt{acc: accFromLines(initLine,
		`{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"fail"},"permission_denials":[{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}]}`)}
	v := classify(a, []byte(verdictSchema), "", false)
	if v.class != classOK || string(v.output) != `{"verdict":"fail"}` {
		t.Fatalf("got %+v", v)
	}
	if d := a.acc.Result.Denials; len(d) != 1 || !strings.HasPrefix(d[0], "Bash: ") {
		t.Fatalf("denials %v", d)
	}
}

func TestDriftedFieldKeepsResult(t *testing.T) {
	a := &attempt{acc: accFromLines(initLine,
		`{"type":"result","subtype":"success","is_error":false,"api_error_status":"none","structured_output":{"verdict":"pass"},"total_cost_usd":0.5}`)}
	v := classify(a, []byte(verdictSchema), "", false)
	if v.class != classOK || a.acc.Result.CostUSD != 0.5 || a.acc.BadLines != 0 {
		t.Fatalf("got %+v, acc %+v", v, a.acc)
	}
}
