package probe

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var stub, _ = filepath.Abs("../claude")

func TestOK(t *testing.T) {
	o, err := Call(stub, []string{"STUB_MODE=ok"}, 10*time.Second)
	if err != nil || o.Kind != OK || o.Output["n"].(float64) != 3 {
		t.Fatalf("%v %v", o, err)
	}
}
func TestNull(t *testing.T) {
	if o, _ := Call(stub, []string{"STUB_MODE=nullout"}, 10*time.Second); o.Kind != Null {
		t.Fatal(o)
	}
}
func TestHangKilled(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pid")
	start := time.Now()
	o, _ := Call(stub, []string{"STUB_MODE=hang", "STUB_PIDFILE=" + f}, time.Second)
	if o.Kind != Timeout || time.Since(start) > 3*time.Second {
		t.Fatal(o, time.Since(start))
	}
	b, _ := os.ReadFile(f)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil {
		t.Fatal("grandchild alive")
	}
}
func TestParallel(t *testing.T) {
	for _, o := range Parallel(stub, 8) {
		if o.Kind != OK {
			t.Fatal(o)
		}
	}
}
