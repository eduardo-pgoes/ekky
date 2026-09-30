export type Span = { code: boolean; text: string }

// Prose carries backtick spans and nothing else; everything between the ticks is code.
export function spans(s: string): Span[] {
  const out: Span[] = []
  const re = /`([^`]+)`/g
  let at = 0
  for (let m = re.exec(s); m; m = re.exec(s)) {
    if (m.index > at) out.push({ code: false, text: s.slice(at, m.index) })
    out.push({ code: true, text: m[1] })
    at = m.index + m[0].length
  }
  if (at < s.length) out.push({ code: false, text: s.slice(at) })
  return out
}

export const paragraphs = (s: string) => s.split(/\n\s*\n/)
