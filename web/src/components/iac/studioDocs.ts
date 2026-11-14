// Splits editor text on `---` lines so the form edits the first document only.
const SEPARATOR = /^---[ \t]*$/

export interface StudioDocs {
  segments: string[]
  firstIndex: number
  count: number
}

export function splitStudioDocs(text: string): StudioDocs {
  const segments: string[][] = [[]]
  for (const line of text.split('\n')) {
    if (SEPARATOR.test(line)) segments.push([])
    else segments[segments.length - 1]?.push(line)
  }
  const joined = segments.map((s) => s.join('\n'))
  const filled = joined.filter((s) => s.trim() !== '')
  return {
    segments: joined,
    firstIndex: joined.findIndex((s) => s.trim() !== ''),
    count: filled.length,
  }
}

export function firstDocument(text: string): string {
  const docs = splitStudioDocs(text)
  return docs.firstIndex < 0 ? '' : (docs.segments[docs.firstIndex] ?? '')
}

// Swaps the first document and keeps every other document byte for byte.
export function replaceFirstDocument(text: string, yaml: string): string {
  const docs = splitStudioDocs(text)
  if (docs.firstIndex < 0) return yaml
  const next = [...docs.segments]
  next[docs.firstIndex] = yaml.replace(/\n+$/, '')
  return next.join('\n---\n')
}
