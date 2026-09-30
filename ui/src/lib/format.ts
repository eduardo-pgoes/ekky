export const money = (n: number) => `$${n.toFixed(2)}`

export function dur(ms: number) {
  const s = Math.round(ms / 1000)
  if (s < 90) return `${s}s`
  const m = Math.round(s / 60)
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h${String(m % 60).padStart(2, '0')}`
}

export function roman(n: number) {
  const t: [number, string][] = [[10, 'x'], [9, 'ix'], [5, 'v'], [4, 'iv'], [1, 'i']]
  let out = ''
  for (const [v, s] of t) while (n >= v) { out += s; n -= v }
  return out
}
