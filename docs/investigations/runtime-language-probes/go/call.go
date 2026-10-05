package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Kind int

const (
	OK Kind = iota
	Null
	Timeout
)

type Outcome struct {
	Kind   Kind
	Output map[string]any
}

func Call(binary string, env []string, timeout time.Duration) (Outcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-p", "x", "--output-format", "stream-json")
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{}, err
	}
	if err := cmd.Start(); err != nil {
		return Outcome{}, err
	}
	res := Outcome{Kind: Null}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		var ev struct {
			Type             string         `json:"type"`
			StructuredOutput map[string]any `json:"structured_output"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Type == "result" && ev.StructuredOutput != nil {
			res = Outcome{OK, ev.StructuredOutput}
		}
	}
	werr := cmd.Wait()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Outcome{Kind: Timeout}, nil
	}
	_ = werr
	return res, nil
}

func Parallel(binary string, n int) []Outcome {
	outs := make([]Outcome, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); outs[i], _ = Call(binary, []string{"STUB_MODE=ok"}, 10*time.Second) }()
	}
	wg.Wait()
	return outs
}
