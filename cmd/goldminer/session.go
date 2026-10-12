package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"goldminer/internal/flash"
	"goldminer/internal/savegame"
	"time"
)

type playerKey struct{ Player, Code int }
type controls struct {
	Keys                            []playerKey
	X, Y                            float64
	Down, Pressed, Released, Escape bool
}

func findClip(n *flash.Node, id int) *flash.Node {
	if n.Def.ID == id {
		return n
	}
	for _, c := range n.Children {
		if v := findClip(c, id); v != nil {
			return v
		}
	}
	return nil
}
func (g *game) inLevel() bool {
	return g.rt.Get(g.rt.Root, "C") != nil && findClip(g.rt.Root, 197) != nil
}
func numeric(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

func (g *game) newCampaign(players int) error {
	seed := g.seed
	g.seed++
	rt, err := flash.New(g.movie, seed)
	if err != nil {
		return err
	}
	if err = rt.SetPlayers(players); err != nil {
		return err
	}
	// The native start buttons use the original Start action's initial values.
	for k, v := range map[string]any{"minerStrength": float64(10), "total": float64(10), "time": float64(60), "goalAddOn": float64(0), "goal": float64(375), "level": float64(0), "stick": float64(0), "score": float64(0), "levelDis": float64(0)} {
		rt.Set(rt.Root, k, v)
	}
	if err = rt.Goto(rt.Root, "start", true); err != nil {
		return err
	}
	id := make([]byte, 12)
	if _, err = rand.Read(id); err != nil {
		return err
	}
	oldRT, oldRecord := g.rt, g.record
	now := time.Now()
	g.rt = rt
	g.record = savegame.Record{Version: 1, ID: hex.EncodeToString(id), CreatedAt: now, Players: players}
	if err = g.saveSession(false); err != nil {
		g.rt = oldRT
		g.record = oldRecord
		return err
	}
	g.stopAudio()
	g.ui = ""
	g.paused = false
	g.resumeLeft = 0
	g.clock = frameClock{}
	g.shopSaved = false
	return nil
}

func (g *game) saveSession(completed bool) error {
	if g.store == nil || g.record.ID == "" {
		return nil
	}
	if g.record.Completed && g.historySaved {
		return nil
	}
	rec := g.record
	if !rec.Completed {
		rec.UpdatedAt = time.Now()
	}
	rec.Players = g.rt.Players()
	rec.Level = int(numeric(g.rt.Get(g.rt.Root, "levelDis")))
	if rec.Level < 1 {
		rec.Level = 1
	}
	rec.Score = numeric(g.rt.Get(g.rt.Root, "score"))
	rec.Goal = numeric(g.rt.Get(g.rt.Root, "goal"))
	rec.Completed = rec.Completed || completed
	if rec.Completed {
		rec.Checkpoint = nil
	} else {
		data, err := g.rt.Checkpoint()
		if err != nil {
			return err
		}
		rec.Checkpoint = data
	}
	if err := g.store.Put(rec); err != nil {
		if rec.Completed {
			g.record = rec // Retry archiving on home/quit after an I/O failure.
		}
		return err
	}
	g.record = rec
	g.historySaved = rec.Completed
	return nil
}
func (g *game) saveOrNotice(completed bool) {
	if err := g.saveSession(completed); err != nil {
		g.notice = "保存失败：" + err.Error()
	} else if g.record.Completed {
		g.notice = "已记入历史记录"
	} else {
		g.notice = "已自动存档"
	}
}
func (g *game) refreshSaves() {
	if g.store == nil {
		return
	}
	var records []savegame.Record
	var err error
	if g.ui == "history" {
		var history []savegame.HistoryRecord
		history, err = g.store.History()
		for _, h := range history {
			records = append(records, savegame.Record{Version: h.Version, ID: h.ID, CreatedAt: h.CreatedAt, UpdatedAt: h.UpdatedAt, Players: h.Players, Level: h.Level, Score: h.Score, Goal: h.Goal, Completed: true})
		}
	} else {
		records, err = g.store.List()
	}
	if err != nil {
		g.notice = "无法读取记录：" + err.Error()
		return
	}
	g.records = records
	if len(g.store.Warnings) > 0 {
		g.notice = fmt.Sprintf("有 %d 个损坏记录已跳过，原文件保留", len(g.store.Warnings))
	}
	maxPage := max(0, (len(records)-1)/6)
	g.page = min(g.page, maxPage)
}
func (g *game) loadCampaign(id string) error {
	g.refreshSaves()
	for _, rec := range g.records {
		if rec.ID != id {
			continue
		}
		if rec.Completed {
			return fmt.Errorf("本局已结束")
		}
		rt, err := flash.Restore(g.movie, rec.Checkpoint)
		if err != nil {
			return fmt.Errorf("存档损坏：%w", err)
		}
		if rt.Players() != rec.Players {
			return fmt.Errorf("存档模式不一致")
		}
		g.stopAudio()
		g.rt = rt
		g.record = rec
		g.historySaved = false
		g.ui = ""
		g.paused = false
		g.resumeLeft = 0
		g.clock = frameClock{}
		g.shopSaved = rt.Root.Frame == 130
		if g.inLevel() {
			g.paused = true
			g.resumeLeft = 3 * inputTPS
		}
		g.notice = "已恢复存档"
		return nil
	}
	return fmt.Errorf("找不到存档")
}
func (g *game) home() {
	g.saveOrNotice(false)
	g.stopAudio()
	g.ui = "home"
	g.paused = false
	g.resumeLeft = 0
	g.refreshSaves()
}
func (g *game) togglePause() {
	if !g.inLevel() {
		return
	}
	if g.resumeLeft > 0 {
		g.resumeLeft = 0
		return
	}
	if g.paused {
		g.resumeLeft = 3 * inputTPS
		return
	}
	g.paused = true
	for _, p := range g.players {
		p.Pause()
	}
}
func (g *game) stopAudio() {
	for _, p := range g.players {
		p.Close()
	}
	g.players = nil
	clear(g.active)
}

// step is also driven by the headless integration tests; it is the same state
// transition path used by real mouse/keyboard events in Update.
func (g *game) step(in controls) error {
	if in.Escape {
		if g.ui != "" {
			g.ui = "home"
		} else {
			g.togglePause()
		}
		g.uiPressed = ""
		return nil
	}
	buttons := g.buttons()
	hit := ""
	for _, b := range buttons {
		if b.enabled && b.bounds.contains(in.X, in.Y) {
			hit = b.id
			break
		}
	}
	if in.Pressed {
		g.uiPressed = hit
	}
	if in.Released {
		pressed := g.uiPressed
		g.uiPressed = ""
		if hit != "" && hit == pressed {
			for _, b := range buttons {
				if b.id == hit {
					g.activate(b.id)
					return nil
				}
			}
		}
	}
	if g.ui != "" {
		return nil
	}
	if g.paused {
		if g.resumeLeft > 0 {
			g.resumeLeft--
			if g.resumeLeft == 0 {
				g.paused = false
				for _, p := range g.players {
					p.Play()
				}
			}
		}
		// The 360th countdown update still cannot move the world. The next update
		// resumes the original fixed-step phase without shortening the countdown.
		return nil
	}
	wasLevel := g.inLevel()
	allowKeys := wasLevel
	if wasLevel && g.rt.Players() == 2 {
		allowKeys = numeric(findClip(g.rt.Root, 197).Get("time")) > 0
	}
	for _, k := range in.Keys {
		if allowKeys {
			if err := g.rt.PlayerKey(k.Player, k.Code); err != nil {
				return err
			}
		}
	}
	x, y, down := in.X, in.Y, in.Down
	if hit != "" || g.uiPressed != "" {
		x, y, down = -100, -100, false
	}
	if err := g.rt.Pointer(x, y, down); err != nil {
		return err
	}
	if g.clock.advance() {
		if err := g.rt.Tick(); err != nil {
			return err
		}
	}
	isLevel := g.inLevel()
	if wasLevel && !isLevel {
		failed := numeric(g.rt.Get(g.rt.Root, "score")) < numeric(g.rt.Get(g.rt.Root, "goal"))
		g.saveOrNotice(failed)
		g.shopSaved = false
		if failed {
			g.ui = "gameover"
			g.stopAudio()
		}
	}
	if !wasLevel && isLevel {
		g.shopSaved = false
		g.saveOrNotice(false)
	}
	if g.rt.Root.Frame == 130 {
		shop := findClip(g.rt.Root, 133)
		if (!g.shopSaved && shop != nil && !shop.Playing) || in.Released {
			g.saveOrNotice(false)
			g.shopSaved = true
		}
	}
	return nil
}

func (g *game) activate(id string) {
	var err error
	switch {
	case id == "solo":
		err = g.newCampaign(1)
	case id == "coop":
		err = g.newCampaign(2)
	case id == "saves" || id == "history":
		g.ui = id
		g.page = 0
		g.refreshSaves()
	case id == "help":
		g.ui = "help"
	case id == "home":
		g.home()
	case id == "pause" || id == "resume":
		g.togglePause()
	case id == "cancel-resume":
		g.resumeLeft = 0
	case id == "quit":
		g.saveOrNotice(false)
		g.done = true
	case id == "prev":
		g.page = max(0, g.page-1)
	case id == "next":
		g.page++
	case len(id) > 5 && id[:5] == "load:":
		err = g.loadCampaign(id[5:])
	case len(id) > 7 && id[:7] == "delete:":
		g.deleteID = id[7:]
		g.deleteFrom = g.ui
		g.ui = "delete"
	case id == "delete-confirm":
		if g.deleteFrom == "history" {
			err = g.store.DeleteHistory(g.deleteID)
		} else {
			err = g.store.Delete(g.deleteID)
		}
		if err == nil && g.record.ID == g.deleteID {
			g.record = savegame.Record{}
		}
		g.deleteID = ""
		g.ui = g.deleteFrom
		g.deleteFrom = ""
		g.refreshSaves()
	case id == "delete-cancel":
		g.deleteID = ""
		g.ui = g.deleteFrom
		g.deleteFrom = ""
	}
	if err != nil {
		if errors.Is(err, savegame.ErrFull) {
			g.notice = "已有 100 个未结束存档，请先在存档列表删除一个。"
		} else {
			g.notice = err.Error()
		}
	}
}
