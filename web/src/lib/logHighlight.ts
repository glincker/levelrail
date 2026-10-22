export interface TextSegment {
  text: string
  match: boolean
}

/** Splits text on case-insensitive occurrences of needle, mirroring filterLogLines' trim and lowercase. */
export function splitByNeedle(text: string, rawNeedle: string): TextSegment[] {
  const needle = rawNeedle.trim().toLowerCase()
  if (needle === '') return [{ text, match: false }]
  const lower = text.toLowerCase()
  const out: TextSegment[] = []
  let from = 0
  for (;;) {
    const at = lower.indexOf(needle, from)
    if (at < 0) break
    if (at > from) out.push({ text: text.slice(from, at), match: false })
    out.push({ text: text.slice(at, at + needle.length), match: true })
    from = at + needle.length
  }
  if (from < text.length) out.push({ text: text.slice(from), match: false })
  return out
}
