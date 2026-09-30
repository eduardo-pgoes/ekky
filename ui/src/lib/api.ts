// The model, as `ekky backlog --json` and `ekky show --json` emit it.
export type Decision = { fork: string; chosen: string; rejected: string; why: string }
export type Result = {
  status: string; summary: string; decisions: Decision[] | null; deviations: string; verify: string; question: string
  cost_usd: number; duration_ms: number; turns: number
}
export type Task = {
  id: string; rfc: string; task: string; title: string
  repo: string; base: string; model: string; account: string; after: string; source: string
  status: string; shown: string; owner: 'me' | 'machine' | 'finished'
  accepted: boolean; live: boolean; upstream: string; descendants: string[] | null; branch: string; pr: string
  result: Result | null
}
export type Detail = { task: Task; record: string; log: string; rfcTitle: string }
export type Tab = 'result' | 'record' | 'log'
export type Row = Task & { depth: number }

export const OWNERS: Task['owner'][] = ['me', 'machine', 'finished']
export const TABS: Tab[] = ['result', 'record', 'log']

export async function fetchBacklog(): Promise<Task[]> {
  const r = await fetch('/api/backlog')
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}

export async function fetchDetail(id: string): Promise<Detail> {
  const r = await fetch(`/api/task/${id}`)
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}
