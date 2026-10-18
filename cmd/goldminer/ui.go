package main

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"goldminer/internal/flash"
	"goldminer/internal/render"
	"image/color"
	"math"
)

type rect struct{ x, y, w, h float64 }

func (r rect) contains(x, y float64) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type uiButton struct {
	id, label string
	bounds    rect
	enabled   bool
}

func button(id, label string, x, y, w, h float64) uiButton {
	return uiButton{id, label, rect{x, y, w, h}, true}
}
func (g *game) buttons() []uiButton {
	switch g.ui {
	case "home":
		return []uiButton{button("solo", "单人开始", 100, 112, 350, 36), button("coop", "双人合作 · 每关 40 秒", 100, 159, 350, 36), button("saves", "继续存档", 100, 206, 167, 36), button("history", "历史记录", 283, 206, 167, 36), button("help", "玩法与操作", 100, 253, 167, 32), button("quit", "退出游戏", 283, 253, 167, 32)}
	case "help":
		return []uiButton{button("home", "返回首页", 190, 330, 170, 32)}
	case "gameover":
		return []uiButton{button("solo", "再玩一局 · 单人", 140, 220, 270, 34), button("coop", "再玩一局 · 双人", 140, 267, 270, 34), button("home", "返回首页", 190, 318, 170, 30)}
	case "delete":
		keep := "保留存档"
		if g.deleteFrom == "history" {
			keep = "保留记录"
		}
		return []uiButton{button("delete-confirm", "确认删除", 105, 235, 160, 36), button("delete-cancel", keep, 285, 235, 160, 36)}
	case "saves", "history":
		out := []uiButton{button("home", "返回", 28, 18, 68, 28)}
		if g.ui == "history" {
			out = append(out, button("saves", "继续存档", 425, 18, 100, 28))
		} else {
			out = append(out, button("history", "历史记录", 425, 18, 100, 28))
		}
		start := g.page * 6
		for i := start; i < min(len(g.records), start+6); i++ {
			rec := g.records[i]
			y := float64(77 + (i-start)*42)
			if g.ui == "saves" {
				out = append(out, button("load:"+rec.ID, "继续", 405, y, 55, 31))
			}
			out = append(out, button("delete:"+rec.ID, "删除", 469, y, 55, 31))
		}
		prev := button("prev", "上一页", 110, 341, 120, 30)
		prev.enabled = g.page > 0
		next := button("next", "下一页", 320, 341, 120, 30)
		next.enabled = (g.page+1)*6 < len(g.records)
		return append(out, prev, next)
	}
	if g.paused {
		if g.resumeLeft > 0 {
			return []uiButton{button("cancel-resume", "保持暂停", 175, 265, 200, 35)}
		}
		return []uiButton{button("resume", "恢复游戏", 175, 195, 200, 38), button("home", "存档并返回首页", 175, 251, 200, 35)}
	}
	if g.inLevel() {
		return []uiButton{button("pause", "暂停", 481, 71, 62, 25)}
	}
	if g.record.ID != "" {
		return []uiButton{button("home", "存档 / 首页", 438, 370, 105, 24)}
	}
	return nil
}

var ink = color.NRGBA{R: 70, G: 43, B: 17, A: 255}
var cream = color.NRGBA{R: 255, G: 249, B: 225, A: 255}

func fill(screen *ebiten.Image, r rect, c color.NRGBA) {
	v := render.Viewport(screen.Bounds().Dx(), screen.Bounds().Dy())
	x, y := v.Point(r.x, r.y)
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(r.w*v.A), float32(r.h*v.D), c, true)
}
func (g *game) label(screen *ebiten.Image, s string, x, y, w, h, size float64, c color.NRGBA, align int) {
	g.renderer.Label(screen, s, x, y, w, h, size, c, align)
}
func (g *game) drawPlayerScores(screen *ebiten.Image, y float64, c color.NRGBA) {
	scores := g.rt.PlayerScores()
	g.label(screen, "左侧：$"+flash.String(scores[0]), 20, y, 200, 24, 12, c, 0)
	g.label(screen, "右侧：$"+flash.String(scores[1]), 225, y, 200, 24, 12, c, 0)
}
func (g *game) drawUI(screen *ebiten.Image) {
	if g.ui != "" {
		fill(screen, rect{0, 0, 550, 400}, color.NRGBA{R: 46, G: 35, B: 20, A: 245})
		fill(screen, rect{15, 10, 520, 377}, cream)
	}
	switch g.ui {
	case "home":
		g.label(screen, "黄金矿工", 60, 24, 430, 50, 28, ink, 2)
		g.label(screen, "中文桌面版 · 自动存档 · 双人合作", 65, 77, 420, 25, 14, ink, 2)
		g.label(screen, "单人 60 秒 / 双人 40 秒；双人分别计分，合计过关", 45, 298, 460, 25, 13, ink, 2)
		g.label(screen, "每关结束自动保存，最多保留 100 个存档", 45, 321, 460, 25, 13, ink, 2)
	case "help":
		g.label(screen, "玩法与操作", 60, 21, 430, 38, 25, ink, 2)
		g.label(screen, "单人：↓ 出钩，↑ 使用炸药，← / → 提前收回。\n双人左侧：S 出钩，W 使用炸药。\n双人右侧：↓ 出钩，↑ 使用炸药，← / → 提前收回。\n双人分别计分，合计金额达到目标即可过关。\n双人共享矿物、炸药和商店加成，每关 40 秒。\n过关后点击商品购买道具，两人平摊费用。\n准备好后点“下一关”，继续抓黄金、钻石或神秘袋。\n点击“暂停”冻结整关；点击恢复后倒数 3 秒再继续。\n每关结束自动存档；返回首页或关闭窗口也会保存。\n继续存档会恢复当时的关卡、道具和商店状态。", 43, 68, 464, 250, 15, ink, 0)
	case "saves", "history":
		title := fmt.Sprintf("存档 %d / 100", len(g.records))
		empty := "还没有存档，先开始一局吧"
		if g.ui == "history" {
			title = fmt.Sprintf("历史记录 · %d 局", len(g.records))
			empty = "已结束的游戏会自动记在这里"
		}
		g.label(screen, title, 102, 15, 312, 35, 21, ink, 2)
		if len(g.records) == 0 {
			g.label(screen, empty, 60, 167, 430, 36, 19, ink, 2)
		}
		for i := g.page * 6; i < min(len(g.records), (g.page+1)*6); i++ {
			rec := g.records[i]
			y := float64(75 + (i-g.page*6)*42)
			mode := "单人"
			if rec.Players == 2 {
				mode = "双人"
			}
			state := "进行中"
			if rec.Completed {
				state = "已结束"
			}
			fill(screen, rect{25, y, 500, 38}, color.NRGBA{R: 247, G: 228, B: 181, A: 255})
			g.label(screen, fmt.Sprintf("%s · 第 %d 关 · $%.0f · %s", mode, rec.Level, rec.Score, state), 33, y, 365, 20, 12, ink, 0)
			g.label(screen, rec.UpdatedAt.Local().Format("2006-01-02 15:04:05"), 33, y+17, 365, 20, 10, ink, 0)
		}
		g.label(screen, fmt.Sprintf("%d / %d", g.page+1, max(1, (len(g.records)+5)/6)), 232, 342, 86, 26, 13, ink, 2)
	case "delete":
		question, detail := "删除这个存档？", "删除后无法继续该局游戏。"
		if g.deleteFrom == "history" {
			question, detail = "删除这条历史记录？", "仅删除这条已结束游戏的记录。"
		}
		g.label(screen, question, 60, 115, 430, 40, 26, ink, 2)
		g.label(screen, detail, 60, 168, 430, 30, 16, ink, 2)
	case "gameover":
		g.label(screen, "游戏结束", 60, 60, 430, 48, 26, ink, 2)
		g.label(screen, fmt.Sprintf("第 %d 关 · 金额 $%.0f / 目标 $%.0f", g.record.Level, g.record.Score, g.record.Goal), 40, 124, 470, 33, 18, ink, 2)
		message := "未达到目标，成绩已记入历史记录。"
		if !g.historySaved {
			message = "未达到目标，历史记录尚未保存，请返回首页重试。"
		}
		g.label(screen, message, 40, 163, 470, 30, 14, ink, 2)
		if g.rt.Players() == 2 {
			g.drawPlayerScores(screen, 193, ink)
		}
	}
	if g.ui == "" && g.paused {
		fill(screen, rect{0, 0, 550, 400}, color.NRGBA{A: 180})
		fill(screen, rect{100, 98, 350, 218}, cream)
		if g.resumeLeft > 0 {
			g.label(screen, "准备继续", 130, 115, 290, 36, 22, ink, 2)
			g.label(screen, fmt.Sprint(int(math.Ceil(float64(g.resumeLeft)/inputTPS))), 150, 164, 250, 88, 48, ink, 2)
		} else {
			g.label(screen, "游戏已暂停", 130, 127, 290, 42, 23, ink, 2)
		}
	}
	for _, b := range g.buttons() {
		c := color.NRGBA{R: 226, G: 161, B: 48, A: 255}
		if !b.enabled {
			c = color.NRGBA{R: 202, G: 193, B: 175, A: 255}
		} else if g.uiPressed == b.id {
			c = color.NRGBA{R: 200, G: 125, B: 20, A: 255}
		}
		fill(screen, b.bounds, c)
		size := 12.0
		if b.bounds.h < 30 {
			size = 10
		}
		g.label(screen, b.label, b.bounds.x, b.bounds.y+2, b.bounds.w, b.bounds.h-2, size, ink, 2)
	}
	if g.ui != "" && g.notice != "" {
		g.label(screen, g.notice, 27, 372, 495, 24, 10, ink, 2)
	}
	if g.ui == "" && g.rt.Root.Frame == 130 {
		g.label(screen, fmt.Sprintf("金额：$%.0f", numeric(g.rt.Get(g.rt.Root, "score"))), 20, 370, 250, 26, 14, cream, 0)
		if g.rt.Players() == 2 {
			g.drawPlayerScores(screen, 341, cream)
		}
	}
	if g.ui == "" && g.inLevel() && g.rt.Players() == 2 {
		fill(screen, rect{8, 70, 420, 24}, cream)
		g.drawPlayerScores(screen, 70, ink)
		g.label(screen, "左：S / W  ·  右：↓ / ↑", 397, 384, 148, 14, 8, cream, 2)
	}
}
