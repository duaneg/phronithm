package lsfrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// reapOrphans removes what a previous launch left running: a SIGKILLed
// launch's calls and gates keep running and would race its successor
// (spike run 4). Each is a container carrying this state dir's label, and
// is removed with podman rm -f. A failed podman ps, or a container not
// removed, is the launch's error: a live container may still write a
// clone or gate checkout this launch would read. A children.json an
// earlier runtime left (gate process markers) is logged and removed, not
// acted on. In a dry run, orphans are reported, not removed.
func (rt *Runtime) reapOrphans() error {
	if err := rt.reapContainers(); err != nil {
		return err
	}
	path := filepath.Join(rt.cfg.StateDir, "children.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var recorded map[string]int
	if err := json.Unmarshal(b, &recorded); err != nil {
		recorded = nil
	}
	rt.Log(fmt.Sprintf("children.json from an earlier runtime (%d bytes, %d entries) dropped, not acted on", len(b), len(recorded)))
	rt.event("children-dropped", map[string]any{"entries": recorded})
	if !rt.cfg.DryRun {
		os.Remove(path)
	}
	return nil
}

// reapContainers removes the containers carrying this state dir's label.
func (rt *Runtime) reapContainers() error {
	ctx, cancel := context.WithTimeout(context.Background(), rmTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, rt.cfg.PodmanBin, "ps", "-a", "--filter", "label="+labelState+"="+rt.stateHash, "--format", "{{.Names}}").Output()
	if err != nil {
		rt.event("orphan-check-failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("orphan check: podman ps: %w%s", err, stderrOf(err))
	}
	for _, name := range strings.Fields(string(out)) {
		if rt.cfg.DryRun {
			rt.event("orphan-found", map[string]any{"container": name})
			rt.Log("dry run: orphaned container " + name + " left running")
			continue
		}
		if err := rt.removeContainer(name); err != nil {
			return fmt.Errorf("orphaned container %s not removed: %w", name, err)
		}
		rt.event("orphan-terminated", map[string]any{"container": name})
		rt.Log("removed orphaned container " + name + " left by a previous launch")
	}
	return nil
}
