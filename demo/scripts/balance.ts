// demo/scripts/balance.ts — 数值验算脚本（无需安装 Node）
// 运行: gnd script run demo/scripts/balance.ts

interface Hero {
  name: string
  hp: number
  atk: number
  def: number
  spd: number
}

function rowToHero(headers: string[], row: string[]): Hero {
  const m: Record<string, string> = {}
  headers.forEach((h, i) => { m[h] = row[i] })
  return {
    name: m["name"] || "?",
    hp: +m["base_hp"] || 0,
    atk: +m["base_atk"] || 0,
    def: +m["base_def"] || 0,
    spd: +m["base_spd"] || 100,
  }
}

const table = gnd.loadTable("demo/configs/HeroConfig.csv", "")
if (!table || table.count === 0) {
  throw new Error("HeroConfig 为空，请先 gnd demo init")
}

const results: any[] = []
for (const row of table.rows as string[][]) {
  const hero = rowToHero(table.headers as string[], row)
  const sim = gnd.simBattle(
    {
      name: hero.name, hp: hero.hp * 8, atk: hero.atk * 8, def: hero.def * 8,
      spd: hero.spd, crit_rate: 0.15, crit_dmg: 0.5, skill_mult: 1.4,
    },
    {
      name: "裂界造物", hp: 9000, atk: 500, def: 2500, spd: 95,
      crit_rate: 0.05, crit_dmg: 0.5, skill_mult: 1,
    },
    150,
    42,
  )
  results.push({
    hero: hero.name,
    win_rate: sim.win_rate,
    avg_turns: sim.avg_turns,
  })
  gnd.log(hero.name, "win", sim.win_rate, "turns", sim.avg_turns)
}

export default {
  script: "balance.ts",
  heroes: results,
}
