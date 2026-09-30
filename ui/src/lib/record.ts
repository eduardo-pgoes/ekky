export type RecordBlock =
  | { kind: 'heading'; text: string }
  | { kind: 'para'; text: string }
  | { kind: 'list'; items: string[] }

export type ParsedRecord = { frontmatter: [string, string][]; blocks: RecordBlock[] }

// task.md is frontmatter plus the intent card: `##` headings, `- ` bullets and paragraphs.
// The `#` title line is dropped — the pane already shows the title.
export function parseRecord(md: string): ParsedRecord {
  const m = md.match(/^---\n([\s\S]*?)\n---\n?([\s\S]*)$/)
  const fm = m ? m[1] : '', body = m ? m[2] : md
  const frontmatter = (fm.split('\n').map((l) => l.match(/^(\w+):\s*(.*?)\s*(?:#.*)?$/)).filter(Boolean) as RegExpMatchArray[])
    .map((p) => [p[1], p[2]] as [string, string])

  const blocks: RecordBlock[] = []
  let list: string[] | null = null
  for (const line of body.split('\n')) {
    const l = line.trimEnd()
    if (/^#\s/.test(l)) continue
    if (/^- /.test(l)) {
      if (!list) { list = []; blocks.push({ kind: 'list', items: list }) }
      list.push(l.slice(2))
      continue
    }
    list = null
    if (/^##\s/.test(l)) blocks.push({ kind: 'heading', text: l.replace(/^##\s*/, '') })
    else if (l.trim()) blocks.push({ kind: 'para', text: l })
  }
  return { frontmatter, blocks }
}
