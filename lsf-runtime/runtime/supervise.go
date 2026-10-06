package lsfrt

import (
	"os"
	"os/signal"
	"syscall"
)

// watchSignals installs the SIGINT/SIGTERM handlers. The first signal sets
// the sticky stop: in-flight calls finish, the next call boundary stops. The
// second and later signals cancel in-flight attempts. It returns the teardown.
func (rt *Runtime) watchSignals() func() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		count := 0
		for {
			select {
			case <-done:
				return
			case s := <-ch:
				reason := StopSigterm
				if s == syscall.SIGINT {
					reason = StopSigint
				}
				count++
				first := count == 1
				rt.mu.Lock()
				stop, set := rt.setStopLocked(reason, "")
				if !first && !rt.killed {
					rt.killed = true
					close(rt.killCh)
				}
				rt.mu.Unlock()
				if set {
					rt.stopEvent(stop)
				}
				if first {
					rt.Log(string(reason) + " received; stopping at the next call boundary (signal again to kill in-flight calls)")
				} else {
					rt.Log(s.String() + " received again; killing in-flight calls")
				}
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
