// Package recipe is the numerical "cookbook": goal → model → parameter ranges.
package recipe

import (
	"fmt"
	"sort"
	"strings"
)

// Recipe is one reusable design pattern.
type Recipe struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Goal     string   `json:"goal"`
	Model    string   `json:"model"`   // curve kind / formula family
	Params   string   `json:"params"`  // human-readable ranges
	Anchors  string   `json:"anchors"` // benchmark guidance
	Pitfalls string   `json:"pitfalls"`
	Tags     []string `json:"tags"`
	Example  string   `json:"example"`
}

// All returns the built-in cookbook (extend here as team learns).
func All() []Recipe {
	return []Recipe{
		{
			ID:       "early-fast-growth",
			Title:    "前 3 天快速成长感",
			Goal:     "新手期感到「一直在变强」，同时中后期不空窗",
			Model:    "piecewise-log",
			Params:   "s1≈1.8, s2≈0.6, breakX≈新手期结束等级（如 30）；或 log 曲线 y=a·ln(1+b·x) 在 x∈[0,3] 日斜率 ≥1.8 倍后期",
			Anchors:  "Lv1→Lv10 所需总经验 ≈ 首日可产出经验 × 1.2",
			Pitfalls: "s1 过大导致第 2 天就顶到玩法解锁墙；break 后 s2 过小出现「升级无感」",
			Tags:     []string{"progress", "retention", "curve"},
			Example:  "gnd curve piecewise-log --s1 1.8 --s2 0.6 --break 30 --max-x 100 --n 50",
		},
		{
			ID:       "combat-damage-physical",
			Title:    "物理减伤公式（通用）",
			Goal:     "攻击成长有收益但不无限穿透",
			Model:    "physical: atk * skill * atk/(atk+def)",
			Params:   "def 与敌方 atk 同量级时减伤约 50%；def >> atk 时收益趋缓（边际递减）",
			Anchors:  "以「基准怪 atk」为 100，小怪 def 20~40，精英 80~120，Boss 150+",
			Pitfalls: "使用 atk-def 线性扣减会在后期出现负伤或完全打不动；双攻公式注意 atk=0 除零",
			Tags:     []string{"combat", "formula"},
			Example:  "gnd combat dmg --atk 1200 --def 400 --skill 1.2 --crit-rate 0.25",
		},
		{
			ID:       "crit-expectation",
			Title:    "暴击期望配平",
			Goal:     "暴击率与暴伤的乘积收益受控",
			Model:    "E = base * (1 + cr*(cm-1))",
			Params:   "常见：cr 0.05→0.75，cm 1.5→2.5；保持 cr*(cm-1) 的期望加成在角色定位区间",
			Anchors:  "主 C：期望暴击加成 30%~60%；辅助：0~20%",
			Pitfalls: "只堆暴伤不堆暴率导致体验方差过大；满暴后再堆暴率无收益需转化机制",
			Tags:     []string{"combat", "stat"},
			Example:  "gnd combat dmg --atk 1000 --def 300 --crit-rate 0.5 --crit-mult 2.0",
		},
		{
			ID:       "economy-ratio-band",
			Title:    "货币健康比",
			Goal:     "避免通胀/通缩崩盘",
			Model:    "prod/cons ratio",
			Params:   "健康带 [0.90, 1.15]；预警 [1.15, 1.30]；>1.30 连续 3 日判崩",
			Anchors:  "按 cohort 分层看（新手 7 日 / 30 日活跃 / 回流），总量比会掩盖分层问题",
			Pitfalls: "只看全服总量；忽略回收活动的一次性消耗造成的假平衡",
			Tags:     []string{"economy", "ops"},
			Example:  "gnd economy balance --name gold --prod 1200 --cons 1000 --stock 50000",
		},
		{
			ID:       "gacha-soft-pity",
			Title:    "软保底曲线",
			Goal:     "降低「吃满硬保底」的挫败感，同时控制成本",
			Model:    "base rate + soft pity ramp to hard pity",
			Params:   "base 0.6%~1.6%；soft start ≈ 0.65×hard；step 使 hard-1 时 rate→100%",
			Anchors:  "P50 出金约 hard/2 附近；P90 明显低于 hard；硬保底占比 <25%",
			Pitfalls: "软保底过晚 → 体感和无保底一样；过早 → 成本模型被击穿",
			Tags:     []string{"gacha", "probability"},
			Example:  "gnd gacha sim --rate 0.006 --hard 90 --soft-start 59 --soft-step 0.03 --pulls 80 --sessions 20000",
		},
		{
			ID:       "stage-difficulty-curve",
			Title:    "关卡难度曲线",
			Goal:     "通关率从教学关到挑战关平滑下探",
			Model:    "clear_rate(x) logistic down",
			Params:   "首关 clear ≥90%；中期主线 40%~70%；挑战关 15%~30%；避免相邻关跳变 >25pp",
			Anchors:  "用「玩家能力分布 P50 角色数值」做自动通关模拟标定",
			Pitfalls: "难度不连续造成流失尖峰；用固定数值墙而不是机制变化",
			Tags:     []string{"progress", "level"},
			Example:  "gnd curve sigmoid --l 1 --k 0.15 --x0 20 --base 0.1 --max-x 60",
		},
		{
			ID:       "sensitivity-first",
			Title:    "调参前先做敏感度",
			Goal:     "把精力花在真正影响体验的参数上",
			Model:    "local finite-difference elasticity",
			Params:   "相对扰动 ±10%；按 |elasticity| 或 influence 排序",
			Anchors:  "Top3 参数才进入人工精调；长尾参数用默认值",
			Pitfalls: "一次改多个参数无法归因；忽略参数交互（需后续加 pairwise）",
			Tags:     []string{"balance", "process"},
			Example:  "（见 gnd balance sensitivity 的 JSON objective 接口 / 测试示例）",
		},
		{
			ID:       "feel-level-up",
			Title:    "升级爽感阈值",
			Goal:     "把「没感觉」变成可验收指标",
			Model:    "adjacent level stat gain %",
			Params:   "关键属性相邻等级增幅 ≥8%；全属性加权增幅建议 8%~35%",
			Anchors:  "配合养成成本曲线，保持 gain/cost 比值相对稳定",
			Pitfalls: "只加面板不加手感相关（攻速/移速/技能段数）；只看攻击忽略有效生命",
			Tags:     []string{"feel", "progress"},
			Example:  "gnd feel decode --metric level_stat_gain_pct --value 5",
		},
	}
}

// Get returns a recipe by id.
func Get(id string) (Recipe, error) {
	for _, r := range All() {
		if r.ID == id {
			return r, nil
		}
	}
	return Recipe{}, fmt.Errorf("recipe %q not found", id)
}

// Search filters by tag or free text.
func Search(query string) []Recipe {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return All()
	}
	var out []Recipe
	for _, r := range All() {
		blob := strings.ToLower(strings.Join([]string{
			r.ID, r.Title, r.Goal, r.Model, r.Params, r.Anchors, r.Pitfalls,
			strings.Join(r.Tags, " "),
		}, " "))
		if strings.Contains(blob, q) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Tags returns unique tags.
func Tags() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range All() {
		for _, t := range r.Tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}
