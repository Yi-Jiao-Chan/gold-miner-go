// Package i18n translates presentation text while preserving the original data.
package i18n

import "strings"

const Font = -1

type Static struct {
	Source string   `json:"source"`
	Text   string   `json:"text"`
	Height float64  `json:"height"`
	Align  int      `json:"align,omitempty"`
	Color  [4]uint8 `json:"color"`
}

func Dynamic(s string) string {
	for _, p := range [][2]string{
		{"After you have grabbed", "炸药：抓住物品后按投掷键，炸毁抓钩上的物品。单人用 ↑；双人左侧用 W，右侧用 ↑，数量共享。"},
		{"Strength drink.", "力量药水：下一关收回物品的速度更快，两位矿工都能受益。仅下一关有效。"},
		{"Lucky Clover.", "幸运草：提高下一关神秘袋开出好东西的概率。仅下一关有效。"},
		{"Rock Collectors book.", "石头收藏指南：下一关石头价值变为三倍。仅下一关有效。"},
		{"Diamond Polish.", "钻石抛光剂：提高下一关钻石的价值。仅下一关有效。"},
	} {
		if strings.HasPrefix(s, p[0]) {
			return p[1]
		}
	}
	if strings.HasPrefix(s, "You scored ") && strings.HasSuffix(s, " Points at Gold Miner!") {
		return "本次黄金矿工得分：" + strings.TrimSuffix(strings.TrimPrefix(s, "You scored "), " Points at Gold Miner!")
	}
	return s
}
