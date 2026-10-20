import type { ImportSource } from '../queries/imports'

export interface ImportExample {
  label: string
  value: string
}

export const IMPORT_EXAMPLES: ImportExample[] = [
  { label: 'GitHub repo', value: 'https://github.com/owner/repo' },
  { label: 'Image', value: 'ghcr.io/owner/app:1.0.0' },
  {
    label: 'docker run',
    value: 'docker run -d -p 8080:80 -e TZ=UTC nginx:1.27',
  },
]

/**
 * A cheap client-side guess at what the operator pasted, shown as a hint
 * while typing. The server's classification is authoritative.
 */
export function guessImportSource(text: string): ImportSource | null {
  const t = text.trim()
  if (!t) return null
  if (/^(sudo\s+)?docker\s+(container\s+)?run\b/.test(t)) return 'docker_run'
  if (/^services:\s*$/m.test(t)) return 'compose'
  if (/^\s*FROM\s+\S+/im.test(t) && (t.includes('\n') || /^FROM\s/i.test(t))) {
    return 'dockerfile'
  }
  if (/\s/.test(t)) return null
  if (
    /^https?:\/\//.test(t) ||
    /^git@[^:]+:/.test(t) ||
    /^(github\.com|gitlab\.com|bitbucket\.org|codeberg\.org)\//.test(t)
  ) {
    return 'repo'
  }
  return /^[a-z0-9][a-z0-9._/-]*(:[\w][\w.-]*)?(@sha256:[a-f0-9]{64})?$/.test(t)
    ? 'image'
    : null
}

export const IMPORT_SOURCE_LABEL: Record<ImportSource, string> = {
  repo: 'Git repository',
  docker_run: 'docker run command',
  image: 'Container image',
  compose: 'Docker Compose file',
  dockerfile: 'Dockerfile',
}
