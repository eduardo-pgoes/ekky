import type { Row, Task } from './api'

// Tree order: roots first, each followed by its descendants, depth from `after:`. The tree is
// computed per RFC over every task regardless of owner, so a chain keeps its depth when it
// straddles `me` and `machine`.
export function orderTree(tasks: Task[]): Row[] {
  const out: Row[] = []
  const rfcs = [...new Set(tasks.map((t) => t.rfc))].sort()
  for (const rfc of rfcs) {
    const list = tasks.filter((t) => t.rfc === rfc).sort((a, b) => a.task.localeCompare(b.task))
    const slugs = new Set(list.map((t) => t.task))
    const kids = new Map<string, Task[]>()
    const roots: Task[] = []
    for (const t of list) {
      if (t.after && slugs.has(t.after)) kids.set(t.after, [...(kids.get(t.after) ?? []), t])
      else roots.push(t)
    }
    const walk = (t: Task, depth: number) => { out.push({ ...t, depth }); for (const k of kids.get(t.task) ?? []) walk(k, depth + 1) }
    roots.forEach((r) => walk(r, 0))
  }
  return out
}
