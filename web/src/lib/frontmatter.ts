// Strips a leading YAML frontmatter block ("---\n...\n---"). Only `title`
// is read: Mintlify keeps the page title there instead of an H1, so it is
// restored as a leading H1 when the body has none.
export function stripFrontmatter(text: string): string {
  if (!text.startsWith('---\n') && !text.startsWith('---\r\n')) return text
  const end = text.indexOf('\n---', 4)
  if (end === -1) return text
  const rest = text.slice(end + 4)
  const body = rest.startsWith('\n') ? rest.slice(1) : rest
  const title = /^title:\s*(?:"((?:[^"\\]|\\.)*)"|'([^']*)'|(.+?))\s*$/m.exec(
    text.slice(0, end),
  )
  const value = (title?.[1] ?? title?.[2] ?? title?.[3])?.replace(/\\"/g, '"')
  if (!value || /^\s*#\s+\S/m.test(body)) return body
  return `# ${value}\n\n${body.replace(/^\n+/, '')}`
}
