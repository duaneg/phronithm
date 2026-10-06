package lsfrt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Parallel runs every thunk and waits for all of them. A branch's non-stop
// error or panic is logged and its slot is nil (the JS parallel's catch). If
// any branch stopped, the first StopError, in thunk order, is returned after
// all branches finish.
func (rt *Runtime) Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, len(thunks))
	errs := make([]error, len(thunks))
	var wg sync.WaitGroup
	for i, th := range thunks {
		wg.Go(func() {
			defer func() {
				if p := recover(); p != nil {
					out[i] = nil
					rt.Log(fmt.Sprintf("parallel branch %d panicked: %v", i, p))
					rt.event("branch-panic", map[string]any{"branch": i, "panic": fmt.Sprint(p)})
				}
			}()
			out[i], errs[i] = th(ctx)
		})
	}
	wg.Wait()
	var stop error
	for i, err := range errs {
		if err == nil {
			continue
		}
		if _, ok := IsStop(err); ok {
			if stop == nil {
				stop = err
			}
			continue
		}
		out[i] = nil
		rt.Log(fmt.Sprintf("parallel branch %d failed: %v", i, err))
	}
	return out, stop
}
