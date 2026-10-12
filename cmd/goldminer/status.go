package main

import (
	"encoding/json"
	"goldminer/internal/flash"
	"os"
	"path/filepath"
)

// Only enabled by an explicit validation flag; normal games write no telemetry.
func (g *game) writeStatus() {
	if g.statusFile == "" {
		return
	}
	clips := map[string]any{}
	for _, name := range []string{"C", "C2", "miner", "miner2", "C.cl", "C2.cl"} {
		if n, ok := g.rt.Get(g.rt.Root, name).(*flash.Node); ok && n != nil {
			clips[name] = map[string]any{"frame": n.Frame, "playing": n.Playing, "matrix": n.Matrix, "rotation": g.rt.Get(n, "_rotation"), "price": g.rt.Get(n, "price"), "busy": g.rt.Get(n, "busy")}
		}
	}
	targets := []any{}
	for i := 1; i <= int(numeric(g.rt.Get(g.rt.Root, "things"))); i++ {
		name := "T" + flash.String(float64(i))
		n, _ := g.rt.Get(g.rt.Root, name).(*flash.Node)
		if n == nil {
			continue
		}
		ob, _ := g.rt.Get(n, "ob").(*flash.Node)
		price := any(nil)
		if ob != nil {
			price = g.rt.Get(ob, "price")
		}
		targets = append(targets, map[string]any{"name": name, "frame": n.Frame, "bounds": g.rt.Bounds(n), "price": price})
	}
	timer := findClip(g.rt.Root, 197)
	remaining := any(nil)
	if timer != nil {
		remaining = g.rt.Get(timer, "time")
	}
	status := map[string]any{"ui": g.ui, "paused": g.paused, "resume_updates": g.resumeLeft, "players": g.rt.Players(), "root_frame": g.rt.Root.Frame, "level": g.rt.Get(g.rt.Root, "levelDis"), "stick": g.rt.Get(g.rt.Root, "stick"), "score": g.rt.Get(g.rt.Root, "score"), "goal": g.rt.Get(g.rt.Root, "goal"), "time": remaining, "record_id": g.record.ID, "completed": g.record.Completed, "notice": g.notice, "clips": clips, "targets": targets}
	b, err := json.Marshal(status)
	if err != nil {
		return
	}
	temp := g.statusFile + ".tmp"
	if err = os.MkdirAll(filepath.Dir(g.statusFile), 0700); err != nil {
		return
	}
	if err = os.WriteFile(temp, b, 0600); err == nil {
		_ = os.Rename(temp, g.statusFile)
	}
}
