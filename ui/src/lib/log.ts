export type LogLine =
  | { kind: 'ekky'; text: string }
  | { kind: 'tool'; ts: string; tool: string; rest: string }
  | { kind: 'plain'; text: string }

// run.log is the harness's own `ekky:` lines interleaved with timestamped tool lines.
export function parseLog(log: string): LogLine[] {
  return log.split('\n').map((l): LogLine => {
    if (l.startsWith('ekky:')) return { kind: 'ekky', text: l }
    const m = l.match(/^(\d\d:\d\d:\d\d)\s+(\w[\w-]*):(.*)$/)
    if (m) return { kind: 'tool', ts: m[1], tool: m[2], rest: m[3] }
    return { kind: 'plain', text: l }
  })
}
