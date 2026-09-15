# gnd — Game Numerical Design CLI

游戏数值设计命令行工具：把数值策划常用的成长曲线、战斗/经济/抽卡模拟、平衡性分析、手感量化、多格式配置表读写与设计菜谱沉淀成可复用、可校验的 CLI。

> Go 1.27+ · Windows / macOS / Linux

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
| `gnd table` | **xlsx/csv/tsv/json/yaml** 跨格式转换、改值、加行、批量 | 配置表工具链 |
| `gnd demo` | 目标驱动脚手架：goal → 英雄/技能/任务/敌人/抽卡表 | 智能配置 |
| `gnd script` | **内嵌 TS/JS**（esbuild+goja，无需装 Node） | 自定义验算 |
| `gnd sim` | 并行战斗模拟（≤1000 并发，每场日志，可查询） | 模拟器 |
| `gnd report` | **HTML 报告**（内联 SVG 曲线/柱状/直方图，默认 .html） | 交付报告 |
| `gnd validate` | 多格式数值表安全检查（溢出/断崖/单调/重复 ID） | 校验脚本 |
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

支持 csv / tsv / json / yaml / xlsx：

```bash
gnd validate table --file levels.csv --monotonic exp,hp --cliff 0.5
gnd validate table --file levels.xlsx --sheet 等级
gnd validate table --file drop.json
```

### 7. 多格式配置表（xlsx / csv / tsv / json / yaml）

统一按「首行表头 + 数据行」处理；xlsx 可用 `--sheet` / `--data-start`（五行表头可设 3=Client 或 5=Server）。

```bash
# 跨格式转换
gnd table convert levels.xlsx levels.csv --sheet 等级
gnd table convert levels.csv levels.json
gnd table convert levels.json levels.yaml

# 查看
gnd table schema levels.xlsx --sheet 等级
gnd table sheets levels.xlsx
gnd table rows levels.csv --count 10
gnd table get levels.xlsx --id 1001 --field atk --sheet 等级

# 改值（默认 dry-run，加 --write 落盘）
gnd table set levels.xlsx --sheet 等级 --id 1001 --field atk --value 800
gnd table set levels.xlsx --sheet 等级 --id 1001 --field atk --value 800 --expected 700 --write

# 加行 / upsert
gnd table add levels.csv --values '{"id":"1002","name":"新怪","hp":"1200"}' --write
gnd table upsert levels.yaml --values '{"id":"1001","atk":"850"}' --write

# 批量（JSON）
# {"changes":[{"id":"1001","field":"atk","value":"800"}, ...],
#  "upserts":[{"id":"2001","name":"Boss","hp":"99999"}]}
gnd table batch levels.xlsx --sheet 等级 --input changes.json --write

# 写到新文件而不是原地
gnd table set levels.csv --id 2 --field exp --value 300 --write --out levels-tuned.csv
```

说明：
- 默认 **dry-run**，只打印 `row/id/field/old/new` 计划；`--write` 才落盘。
- `--expected` 做乐观锁：当前值不符则拒绝写入。
- JSON/YAML 保存为 `{"headers":[...],"rows":[[...]]}`，保证列序稳定；加载时兼容「对象数组」。
- 所有单元格按字符串处理，避免大整数 ID 精度丢失。

### 8. 菜谱 & NDD

```bash
gnd recipe list
gnd recipe search gacha
gnd recipe show early-fast-growth

gnd ndd new --name "装备强化" --system economy --out ndd-equip.md
```

### 9. Demo：目标驱动配置（崩铁式）

```bash
gnd demo init --dir demo --xlsx          # 生成 goal.yaml + 英雄/技能/任务/敌人/抽卡表
# 改 demo/goal.yaml 里的目标，再：
gnd demo check --goal demo/goal.yaml
gnd demo sim   --goal demo/goal.yaml
```

生成表：`HeroConfig` / `HeroLvUpConfig` / `SkillConfig` / `EnemyConfig` / `TaskConfig` / `ItemConfig` / `GachaPoolConfig`（均为虚构脱敏数据）。

### 10. 内嵌 TypeScript 脚本（无需 Node）

```bash
gnd script run demo/scripts/balance.ts
gnd script eval 'export default gnd.curve("linear", 10, {a:1,b:2})'
```

脚本内全局 `gnd`：`curve` / `damage` / `simBattle` / `loadTable` / `readJSON` / `writeJSON` / `log`。

### 11. 并行战斗模拟器

```bash
# 默认每次 run 前清空 .gnd/sim 下历史；每场独立 JSON 日志；并发 ≤1000
gnd sim run --n 1000 --workers 32 --name pvp-check \
  --a-hp 8000 --a-atk 1200 --d-hp 7500 --d-atk 1100

gnd sim run --n 500 --from-config demo/configs \
  --hero-id 1001 --enemy-id 2020 --level 20

gnd sim list
gnd sim show <run_id>
gnd sim battle <run_id> 1
gnd sim logs <run_id>
gnd sim clear
```

- 内存：结果只聚合计数/直方图，日志直接落盘，不驻留全部战斗数据。
- 历史查询：`.gnd/sim/index.json` + `manifest.json`。
- `.gitignore` 已忽略 `.gnd/` 与战斗日志。

### 12. HTML 报告（默认交付格式）

自包含单文件，内联 SVG，无外链，浏览器直接打开。

```bash
# Demo 全量报告：成长曲线 / 节奏 / 战斗胜率 / 软保底 / 配置表 / 告警
gnd report demo --goal demo/goal.yaml --out demo/report.html

# 模拟 run 报告（sim run 结束也会自动写 report.html）
gnd sim run --n 500 --name pvp
gnd report sim --root .gnd/sim                  # 最新 run
gnd report sim --run run_xxx --sample 30

# 单独画成长曲线
gnd report curve --kind piecewise-log --s1 1.8 --s2 0.6 --break 20 --out curve.html
gnd demo check --goal demo/goal.yaml --html demo/report.html
```

图表包括：每级经验、累计经验、属性倍率、升级天数 vs 目标、精英/Boss 胜率柱状图、软保底概率曲线、模拟回合直方图、KPI 卡片。

## 设计原则（内置在工具里）

1. **锚点必须有依据** — `gnd balance anchors` 对无 rationale 的锚点直接告警。
2. **看分布，不只看期望** — 抽卡/战斗都跑蒙特卡洛，输出 P50/P90。
3. **手感可验收** — 模糊描述翻译成 metric + 健康带，上线前可打勾。
4. **极端安全** — 数值表检查溢出、断崖、单调破坏、死资源货币。
5. **调参先敏感度** — 把精调精力放在 influence Top 参数上。
6. **改表先 dry-run** — `gnd table` 默认只出计划，显式 `--write` 才改文件。

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
  tablekit/        xlsx/csv/tsv/json/yaml 读写与转换
  goal/            目标 → 配置表生成
  script/          内嵌 TS/JS（esbuild+goja）
  simulator/       并行战斗模拟器 + 日志/索引
  htmlreport/      HTML 报告 + 内联 SVG 图表
  validate/        数值表校验
  recipe/          菜谱库
  report/          表格 / JSON / sparkline 输出
  cli/             子命令
demo/              脱敏 demo 配置与脚本
```

## 开发

```bash
go test ./...
go build -o gnd ./cmd/gnd
```

## License

MIT
