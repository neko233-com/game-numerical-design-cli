# gnd — Game Numerical Design CLI

游戏数值设计命令行工具：把数值策划常用的成长曲线、战斗/经济/抽卡模拟、平衡性分析、手感量化与设计菜谱沉淀成可复用、可校验的 CLI。

> Go 1.27+ · 零第三方依赖 · Windows / macOS / Linux

## 安装

```bash
go install github.com/neko233-com/game-numerical-design-cli/cmd/gnd@latest
# 或源码构建
go build -o gnd ./cmd/gnd
```

## 能力一览

| 命令 | 做什么 | 对应方法论 |
|------|--------|------------|
| `gnd curve` | 成长曲线采样、增长率、平滑度（断崖/平坦） | 公式库 & 模型库 |
| `gnd combat` | 期望伤害、EHP、1v1 蒙特卡洛模拟 | 战斗数值 / 模拟器 |
| `gnd economy` | 产出消耗比、存量推演、升级节奏、闭环泄漏 | 经济系统 |
| `gnd gacha` | 软/硬保底分布模拟、P50/P90、E[pulls]、软保底建议 | 概率设计 |
| `gnd balance` | 锚点依据校验、敏感度排序、维度打分、胜率对照 | 平衡性分析框架 |
| `gnd feel` | 「打击感太轻」等模糊描述 ↔ 量化阈值 | 手感量化 |
| `gnd validate` | CSV 数值表安全检查（溢出/断崖/单调/重复 ID） | 校验脚本 |
| `gnd recipe` | 数值菜谱：目标 → 模型 → 参数区间 → 坑 | 菜谱库 |
| `gnd ndd` | 生成数值设计文档（NDD）骨架 + 评审 checklist | 流程沉淀 |

## 快速上手

### 1. 成长曲线

```bash
# 前 3 天快、后期缓（分段对数）
gnd curve piecewise-log --s1 1.8 --s2 0.6 --break 30 --max-x 100 --n 50 --smooth

# S 曲线（有上限）
gnd curve sigmoid --l 100 --k 0.25 --x0 40 --base 10 --max-x 80 --n 40

# 导出 CSV 给表格/引擎
gnd curve logarithmic --a 50 --b 0.08 --max-x 60 --n 60 --format csv > curve.csv
```

### 2. 战斗模拟

```bash
# 期望伤害（含暴击期望）
gnd combat dmg --atk 1200 --def 400 --skill 1.2 --crit-rate 0.25 --crit-mult 1.8

# 1v1 胜率 / 回合分布
gnd combat simulate \
  --a-hp 6000 --a-atk 900 --a-def 250 --a-crit-rate 0.3 \
  --d-hp 5500 --d-atk 850 --d-def 280 \
  --n 5000 --seed 42
```

### 3. 经济

```bash
# 产出/消耗健康度
gnd economy balance --name gold --prod 1300 --cons 1000

# 14 日存量推演（含衰减）
gnd economy project --prod 1200 --cons 1000 --stock 50000 --periods 14 --decay 0.02

# 升级节奏 vs 设计目标
gnd economy pace --daily-exp 800 --levels 100,300,800,2000 --targets 0.5,1,2,4
```

### 4. 抽卡

```bash
# 软保底建议
gnd gacha suggest --rate 0.006 --hard 90

# 分布体验（不是只看期望）
gnd gacha sim --rate 0.006 --hard 90 --soft-start 59 --soft-step 0.03 \
  --pulls 80 --sessions 20000 --seed 1
```

### 5. 手感量化

```bash
gnd feel list
gnd feel encode --phrase "打击感太轻"
gnd feel decode --metric level_stat_gain_pct --value 5
gnd feel decode --readings "daily_prod_cons_ratio=1.5,hit_stop_frames=4"
```

### 6. 数值表校验

```csv
# levels.csv
id,name,exp,hp
1,Lv1,100,500
2,Lv2,220,1100
3,Lv3,500,2500
```

```bash
gnd validate table --file levels.csv --monotonic exp,hp --cliff 0.5
```

### 7. 菜谱 & NDD

```bash
gnd recipe list
gnd recipe search gacha
gnd recipe show early-fast-growth

gnd ndd new --name "装备强化" --system economy --out ndd-equip.md
```

## 设计原则（内置在工具里）

1. **锚点必须有依据** — `gnd balance anchors` 对无 rationale 的锚点直接告警。
2. **看分布，不只看期望** — 抽卡/战斗都跑蒙特卡洛，输出 P50/P90。
3. **手感可验收** — 模糊描述翻译成 metric + 健康带，上线前可打勾。
4. **极端安全** — 数值表检查溢出、断崖、单调破坏、死资源货币。
5. **调参先敏感度** — 把精调精力放在 influence Top 参数上。

## 项目结构

```
cmd/gnd/           入口
internal/
  curves/          成长曲线模型
  combat/          伤害公式 + 战斗模拟
  economy/         经济平衡 / 推演 / 节奏 / 闭环
  gacha/           抽卡分布与保底
  balance/         锚点 / 敏感度 / 维度
  feel/            手感量化表
  validate/        数值表校验
  recipe/          菜谱库
  report/          表格 / JSON / sparkline 输出
  cli/             子命令
```

## 开发

```bash
go test ./...
go build -o gnd ./cmd/gnd
```

## License

MIT
