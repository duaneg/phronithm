package orchtest

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func agent(s *Stub, label, prompt string) json.RawMessage {
	cr, _ := s.Call(context.Background(), prompt, lsfrt.Opts{Label: label})
	return cr.Value
}

func TestRepeatedLabelAndMissingHandlerFail(t *testing.T) {
	s := New(nil, TwoWaves(), nil, nil)
	agent(s, "ledger:w1", "p")
	agent(s, "ledger:w1", "p")
	agent(s, "nosuch:x", "p")
	if len(s.Failures) != 2 || !strings.Contains(s.Failures[0], `"ledger:w1" repeated`) || !strings.Contains(s.Failures[1], "no stub for nosuch:x") {
		t.Errorf("%q", s.Failures)
	}
}

func TestCallStripsSuffix(t *testing.T) {
	s := New(nil, TwoWaves(), nil, nil)
	agent(s, "ledger:w1#2", "p")
	if c := s.Calls[0]; c.Prefix != "ledger" || c.Unit != "w1" {
		t.Errorf("%+v", c)
	}
}

func TestMergeOrderAndCompleted(t *testing.T) {
	s := New(nil, TwoParallel(), map[string]Handler{"merge": func(c Call, st *State) any {
		return Ret(M{"merges": []M{{"unit": "A", "status": "clean"}, {"unit": "B", "status": "unresolvable"}}})
	}}, nil)
	agent(s, "merge:w1", "order:\n- A: branch a (x)\n- B: branch b (y)\n")
	if !slices.Equal(s.State.MergeOrder, []string{"A", "B"}) || !s.State.Completed["A"] || s.State.Completed["B"] {
		t.Errorf("%v %v", s.State.MergeOrder, s.State.Completed)
	}
}

func TestParallel(t *testing.T) {
	stop := &lsfrt.StopError{Reason: lsfrt.StopSigint}
	for _, concurrent := range []bool{false, true} {
		s := New(nil, TwoWaves(), nil, nil)
		s.Concurrent = concurrent
		out, err := s.Parallel(context.Background(), []func(context.Context) (json.RawMessage, error){
			func(context.Context) (json.RawMessage, error) { return json.RawMessage("1"), nil },
			func(context.Context) (json.RawMessage, error) { return json.RawMessage("2"), errors.New("boom") },
			func(context.Context) (json.RawMessage, error) { panic("bang") },
			func(context.Context) (json.RawMessage, error) { return nil, stop },
		})
		if err != stop || string(out[0]) != "1" || out[1] != nil || out[2] != nil {
			t.Errorf("concurrent %v: %q %v", concurrent, out, err)
		}
		if len(s.Failures) != 1 || !strings.Contains(s.Failures[0], "panicked: bang") {
			t.Errorf("concurrent %v: %q", concurrent, s.Failures)
		}
	}
	s := New(nil, TwoWaves(), nil, nil)
	s.ExpectPanic = true
	s.Parallel(context.Background(), []func(context.Context) (json.RawMessage, error){func(context.Context) (json.RawMessage, error) { panic("x") }})
	if len(s.Failures) != 0 {
		t.Error(s.Failures)
	}
}

func TestToBlockLoadsAsGraph(t *testing.T) {
	// Defaults' copy stage serves ToBlock; check the fixture round-trips.
	if b := ToBlock(TwoWaves()); !strings.Contains(b, "    dependencies: [\"A\"]") || !strings.HasSuffix(b, "execution_order: [\"A\", \"B\"]\n```") {
		t.Error(b)
	}
}
