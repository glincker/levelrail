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

export interface StyledPiece<S> {
  text: string
  style: S
  match: boolean
}

/** Marks needle matches on the joined plain text while keeping each piece's own style. */
export function highlightStyled<S>(
  segments: readonly { text: string; style: S }[],
  rawNeedle: string,
): StyledPiece<S>[] {
  const plain = segments.map((s) => s.text).join('')
  const hits = splitByNeedle(plain, rawNeedle)
  const out: StyledPiece<S>[] = []
  let si = 0
  let used = 0
  for (const hit of hits) {
    let left = hit.text.length
    while (left > 0 && si < segments.length) {
      const seg = segments[si]
      if (!seg) break
      const take = Math.min(left, seg.text.length - used)
      out.push({
        text: seg.text.slice(used, used + take),
        style: seg.style,
        match: hit.match,
      })
      used += take
      left -= take
      if (used >= seg.text.length) {
        si++
        used = 0
      }
    }
  }
  return out
}
