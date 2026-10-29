export type PaletteModeKind = 'docs' | 'assistant' | 'docker' | 'tags'

export interface PaletteMode {
  kind: PaletteModeKind
  /** What was typed after the prefix. */
  term: string
}

const DOCKER_WORD = /^(?:docker|deploy)\s+(.*)$/i
const DOCKER_PLUS = /^\+\s*(.*)$/

/** Reads a search prefix: "/" docs, "@" assistant, "#" tags and environments, "docker x", "deploy x" or "+x" Docker Hub. */
export function parseMode(text: string): PaletteMode | null {
  const t = text.trimStart()
  if (t.startsWith('/')) return { kind: 'docs', term: t.slice(1).trim() }
  if (t.startsWith('@')) return { kind: 'assistant', term: t.slice(1).trim() }
  if (t.startsWith('#')) return { kind: 'tags', term: t.slice(1).trim() }
  const docker = DOCKER_WORD.exec(t) ?? DOCKER_PLUS.exec(t)
  if (docker) return { kind: 'docker', term: (docker[1] ?? '').trim() }
  return null
}
