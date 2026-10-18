package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"goldminer/assets"
	"goldminer/internal/flash"
	"goldminer/internal/savegame"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testSession(t *testing.T) *game {
	t.Helper()
	raw, e := fs.ReadFile(assets.Files, "movie.json")
	if e != nil {
		t.Fatal(e)
	}
	var m flash.Movie
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	rt, e := flash.New(&m, 1)
	if e != nil {
		t.Fatal(e)
	}
	store, e := savegame.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return &game{rt: rt, movie: &m, store: store, ui: "home", seed: 32}
}
func updates(t *testing.T, g *game, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if e := g.step(controls{X: -100, Y: -100}); e != nil {
			t.Fatal(e)
		}
	}
}
func click(t *testing.T, g *game, id string) {
	t.Helper()
	for _, b := range g.buttons() {
		if b.id == id && b.enabled {
			x, y := b.bounds.x+b.bounds.w/2, b.bounds.y+b.bounds.h/2
			if e := g.step(controls{X: x, Y: y, Down: true, Pressed: true}); e != nil {
				t.Fatal(e)
			}
			if e := g.step(controls{X: x, Y: y, Released: true}); e != nil {
				t.Fatal(e)
			}
			return
		}
	}
	t.Fatalf("missing button %s ui=%s", id, g.ui)
}
func TestPauseFreezesEntireWorldAndButtonResumesAfterThreeSeconds(t *testing.T) {
	g := testSession(t)
	click(t, g, "coop")
	updates(t, g, 300)
	if !g.inLevel() || numeric(g.rt.Get(g.rt.Root, "goal")) != 650 {
		t.Fatal("new campaign did not initialize")
	}
	if e := g.step(controls{Keys: []playerKey{{2, 15}}}); e != nil {
		t.Fatal(e)
	}
	updates(t, g, 40)
	click(t, g, "pause")
	before, e := g.rt.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	updates(t, g, 5*inputTPS)
	after, _ := g.rt.Checkpoint()
	if !bytes.Equal(before, after) {
		t.Fatal("world moved while paused")
	}
	click(t, g, "resume")
	if g.resumeLeft != 360 {
		t.Fatalf("countdown=%d", g.resumeLeft)
	}
	for i := 0; i < 360; i++ {
		if e := g.step(controls{Keys: []playerKey{{1, 15}, {2, 14}}}); e != nil {
			t.Fatal(e)
		}
		after, _ = g.rt.Checkpoint()
		if !bytes.Equal(before, after) {
			t.Fatalf("world moved during countdown at %d", i)
		}
	}
	if g.paused || g.resumeLeft != 0 {
		t.Fatal("countdown did not finish")
	}
	updates(t, g, 8)
	after, _ = g.rt.Checkpoint()
	if bytes.Equal(before, after) {
		t.Fatal("world did not resume")
	}
	click(t, g, "pause")
	click(t, g, "resume")
	updates(t, g, 100)
	click(t, g, "cancel-resume")
	updates(t, g, 400)
	if !g.paused || g.resumeLeft != 0 {
		t.Fatal("cancel countdown resumed world")
	}
	if e := g.step(controls{Escape: true}); e != nil {
		t.Fatal(e)
	}
	if g.resumeLeft != 360 {
		t.Fatalf("Escape countdown shorter: %d", g.resumeLeft)
	}

}
func TestAutomaticShopSavePurchasesRestartAndNextLevel(t *testing.T) {
	g := testSession(t)
	click(t, g, "coop")
	updates(t, g, 300)
	timer := findClip(g.rt.Root, 197)
	for i, name := range []string{"bonus", "bonus2"} {
		bonus := g.rt.Get(g.rt.Root, name).(*flash.Node)
		g.rt.Set(bonus, "bscore", float64(600-i*200))
		if err := g.rt.Goto(bonus, 2, true); err != nil {
			t.Fatal(err)
		}
	}
	updates(t, g, 240)
	if g.rt.PlayerScores() != [2]float64{600, 400} {
		t.Fatalf("individual earnings=%v", g.rt.PlayerScores())
	}
	g.rt.Set(timer, "time", float64(1))
	updates(t, g, 1500)
	if g.rt.Root.Frame != 130 || g.record.ID == "" {
		t.Fatalf("combined earnings did not pass the level: frame=%d", g.rt.Root.Frame)
	}
	saved, e := g.store.List()
	if e != nil || len(saved) != 1 || saved[0].Completed {
		t.Fatal("missing unfinished automatic save")
	}
	score := numeric(g.rt.Get(g.rt.Root, "score"))
	for _, in := range []controls{{X: 57, Y: 257}, {X: 57, Y: 257, Down: true, Pressed: true}, {X: 57, Y: 257, Released: true}} {
		if e = g.step(in); e != nil {
			t.Fatal(e)
		}
	}
	if numeric(g.rt.Get(g.rt.Root, "score")) >= score {
		t.Fatal("shop click did not buy")
	}
	halfCost := (score - numeric(g.rt.Get(g.rt.Root, "score"))) / 2
	wantScores := [2]float64{600 - halfCost, 400 - halfCost}
	if g.rt.PlayerScores() != wantScores {
		t.Fatalf("purchase balances=%v want=%v", g.rt.PlayerScores(), wantScores)
	}
	saved, _ = g.store.List()
	expected, _ := g.rt.Checkpoint()
	restart := testSession(t)
	restart.store = g.store
	restart.refreshSaves()
	if e = restart.loadCampaign(saved[0].ID); e != nil {
		t.Fatal(e)
	}
	if restart.paused || restart.resumeLeft != 0 {
		t.Fatal("shop restore started a level countdown")
	}
	actual, _ := restart.rt.Checkpoint()
	if !bytes.Equal(expected, actual) {
		t.Fatal("restart rerolled shop or lost purchase")
	}
	// Use original next-level button geometry (shop button81).
	next := findClip(restart.rt.Root, 81)
	if next == nil {
		t.Fatal("next-level button missing")
	}
	bounds := restart.rt.Bounds(next)
	x, y := bounds.X+bounds.W/2, bounds.Y+bounds.H/2
	for _, in := range []controls{{X: x, Y: y}, {X: x, Y: y, Down: true, Pressed: true}, {X: x, Y: y, Released: true}} {
		if e = restart.step(in); e != nil {
			t.Fatal(e)
		}
	}
	updates(t, restart, 400)
	if !restart.inLevel() || restart.rt.Players() != 2 || numeric(restart.rt.Get(restart.rt.Root, "levelDis")) != 2 {
		t.Fatalf("next level did not restore coop: frame=%d level=%v", restart.rt.Root.Frame, restart.rt.Get(restart.rt.Root, "levelDis"))
	}
	if numeric(findClip(restart.rt.Root, 197).Get("time")) > 40 {
		t.Fatal("next level did not use forty seconds")
	}
	if restart.rt.PlayerScores() != wantScores {
		t.Fatalf("next level reset individual scores=%v want=%v", restart.rt.PlayerScores(), wantScores)
	}
}
func TestFailedCampaignMovesToHistoryWithoutLeavingSave(t *testing.T) {
	g := testSession(t)
	click(t, g, "solo")
	updates(t, g, 300)
	g.rt.Set(findClip(g.rt.Root, 197), "time", float64(1))
	updates(t, g, 240)
	if g.ui != "gameover" || !g.record.Completed || !g.historySaved {
		t.Fatal("failed campaign did not enter history")
	}
	id := g.record.ID
	if e := g.loadCampaign(id); e == nil {
		t.Fatal("history can continue")
	}
	active, err := g.store.List()
	if err != nil || len(active) != 0 {
		t.Fatal("finished campaign still occupies a save")
	}
	history, err := g.store.History()
	if err != nil || len(history) != 1 || history[0].ID != id {
		t.Fatal("missing history")
	}
	finishedAt := history[0].UpdatedAt
	click(t, g, "home")
	click(t, g, "saves")
	if len(g.records) != 0 {
		t.Fatal("finished game visible in saves")
	}
	click(t, g, "history")
	if len(g.records) != 1 || g.records[0].ID != id {
		t.Fatal("history row missing")
	}
	for _, b := range g.buttons() {
		if strings.HasPrefix(b.id, "load:") {
			t.Fatal("history offers resume")
		}
	}
	history, _ = g.store.History()
	if !history[0].UpdatedAt.Equal(finishedAt) {
		t.Fatal("returning home changed end time")
	}
	click(t, g, "delete:"+id)
	click(t, g, "delete-cancel")
	if g.ui != "history" || len(g.records) != 1 {
		t.Fatal("cancel did not return to history")
	}
	click(t, g, "delete:"+id)
	click(t, g, "delete-confirm")
	if g.ui != "history" || len(g.records) != 0 {
		t.Fatal("history delete failed")
	}
	click(t, g, "home")
	click(t, g, "quit")
	history, _ = g.store.History()
	active, _ = g.store.List()
	if len(history) != 0 || len(active) != 0 {
		t.Fatal("deleted history recreated on exit")
	}
}

func TestActiveSaveDeleteCannotReappearOnExit(t *testing.T) {
	g := testSession(t)
	click(t, g, "solo")
	click(t, g, "home")
	click(t, g, "saves")
	id := g.record.ID
	click(t, g, "delete:"+id)
	click(t, g, "delete-cancel")
	if g.ui != "saves" || len(g.records) != 1 {
		t.Fatal("cancel deleted active save")
	}
	click(t, g, "delete:"+id)
	click(t, g, "delete-confirm")
	click(t, g, "home")
	click(t, g, "quit")
	active, _ := g.store.List()
	if len(active) != 0 {
		t.Fatal("deleted active save recreated")
	}
}

func TestHistoryWriteFailureKeepsSaveAndRetriesCompletion(t *testing.T) {
	g := testSession(t)
	click(t, g, "solo")
	updates(t, g, 300)
	blocked := filepath.Join(g.store.Dir, "history")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	g.rt.Set(findClip(g.rt.Root, 197), "time", float64(1))
	updates(t, g, 240)
	if g.ui != "gameover" || !g.record.Completed || g.historySaved || !strings.Contains(g.notice, "失败") {
		t.Fatal("archive failure lost completion intent")
	}
	finishedAt := g.record.UpdatedAt
	active, err := g.store.List()
	if err != nil || len(active) != 1 {
		t.Fatal("failed archive discarded recovery save")
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	click(t, g, "home")
	if !g.historySaved || len(g.records) != 0 {
		t.Fatal("home did not retry completion and remove active save")
	}
	history, err := g.store.History()
	if err != nil || len(history) != 1 || !history[0].UpdatedAt.Equal(finishedAt) {
		t.Fatal("retry lost history or changed end time")
	}
}

func TestFullUnfinishedSaveListBlocksNewGameAndPaginates(t *testing.T) {
	g := testSession(t)
	data, e := g.rt.Checkpoint()
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	for i := 0; i < 100; i++ {
		e = g.store.Put(savegame.Record{Version: 1, ID: fmt.Sprintf("campaign-%03d", i), CreatedAt: now, UpdatedAt: now.Add(time.Duration(i) * time.Second), Players: 1, Level: 1, Score: 0, Goal: 650, Checkpoint: data})
		if e != nil {
			t.Fatal(e)
		}
	}
	original := g.rt
	click(t, g, "solo")
	if g.rt != original || g.ui != "home" || !strings.Contains(g.notice, "100") {
		t.Fatal("full saves did not block new campaign clearly")
	}
	click(t, g, "saves")
	for i := 0; i < 16; i++ {
		click(t, g, "next")
	}
	if g.page != 16 {
		t.Fatal("last save page unreachable")
	}
	loads := 0
	for _, b := range g.buttons() {
		if strings.HasPrefix(b.id, "load:") {
			loads++
		}
		if b.id == "next" && b.enabled {
			t.Fatal("extra page offered")
		}
	}
	if loads != 4 {
		t.Fatalf("last page has %d saves", loads)
	}
}

func TestExpiredCoopDoesNotStartAnotherClaw(t *testing.T) {
	g := testSession(t)
	click(t, g, "coop")
	updates(t, g, 300)
	g.rt.Set(findClip(g.rt.Root, 197), "time", float64(0))
	if e := g.step(controls{Keys: []playerKey{{1, 15}, {2, 15}}}); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"C.cl", "C2.cl"} {
		n := g.rt.Get(g.rt.Root, name).(*flash.Node)
		if n.Playing {
			t.Fatal("expired coop allowed a new drop")
		}
	}
}

func TestLoadingPausedLevelCountsDownWithoutMovingWorld(t *testing.T) {
	for _, players := range []int{1, 2} {
		t.Run(fmt.Sprint(players), func(t *testing.T) {
			g := testSession(t)
			if err := g.newCampaign(players); err != nil {
				t.Fatal(err)
			}
			updates(t, g, 300)
			if err := g.step(controls{Keys: []playerKey{{1, 15}}}); err != nil {
				t.Fatal(err)
			}
			updates(t, g, 40)
			click(t, g, "pause")
			click(t, g, "home")
			expected, err := g.rt.Checkpoint()
			if err != nil {
				t.Fatal(err)
			}
			restart := testSession(t)
			restart.store, err = savegame.Open(g.store.Dir)
			if err != nil {
				t.Fatal(err)
			}
			click(t, restart, "saves")
			click(t, restart, "load:"+g.record.ID)
			if !restart.paused || restart.resumeLeft != 360 {
				t.Fatalf("restore immediately resumed: paused=%v countdown=%d", restart.paused, restart.resumeLeft)
			}
			for i := 0; i < 360; i++ {
				if err := restart.step(controls{Keys: []playerKey{{1, 15}}, X: -100, Y: -100}); err != nil {
					t.Fatal(err)
				}
				actual, _ := restart.rt.Checkpoint()
				if !bytes.Equal(expected, actual) {
					t.Fatalf("restore world moved on countdown update %d", i)
				}
				if i < 359 && !restart.paused {
					t.Fatal("countdown ended early")
				}
			}
			if restart.paused || restart.resumeLeft != 0 {
				t.Fatal("restore countdown did not finish")
			}
			updates(t, restart, 8)
			actual, _ := restart.rt.Checkpoint()
			if bytes.Equal(expected, actual) {
				t.Fatal("restored world did not resume")
			}
		})
	}
}
