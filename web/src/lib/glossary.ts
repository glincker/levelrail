export interface GlossaryEntry {
  key: string
  label: string
  body: string
}

// A to Z by label, filtered by a case-insensitive match on label or definition.
export function filterGlossary(
  entries: GlossaryEntry[],
  query: string,
): GlossaryEntry[] {
  const q = query.trim().toLowerCase()
  return entries
    .filter(
      (e) =>
        q === '' ||
        e.label.toLowerCase().includes(q) ||
        e.body.toLowerCase().includes(q),
    )
    .sort((a, b) => a.label.localeCompare(b.label))
}
