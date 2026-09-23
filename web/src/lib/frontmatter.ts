// Strips a leading VitePress/Jekyll-style YAML frontmatter block
// ("---\n...\n---"). Values inside are never read: this docs set only
// uses frontmatter for a page `description` meta tag, meaningless
// in-app.
export function stripFrontmatter(text: string): string {
  if (!text.startsWith('---\n') && !text.startsWith('---\r\n')) return text
  const end = text.indexOf('\n---', 4)
  if (end === -1) return text
  const rest = text.slice(end + 4)
  return rest.startsWith('\n') ? rest.slice(1) : rest
}
