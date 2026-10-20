import type { BrandIconName } from '../components/BrandIcon'

// Icon matching only, never a trust decision: the parsed hostname has to
// equal a known host, so "github.com.evil.test" gets no icon.
const GIT_HOST_ICONS: Record<string, BrandIconName> = {
  'github.com': 'github',
  'gitlab.com': 'gitlab',
  'bitbucket.org': 'bitbucket',
}

// gitHostname returns the host of a URL (scheme optional) or an
// scp-style "git@host:owner/repo" remote, or null when it cannot parse.
export function gitHostname(repoUrl: string): string | null {
  const value = repoUrl.trim()
  const scp = /^[\w.-]+@([\w.-]+):(?!\/)/.exec(value)
  if (scp?.[1]) return scp[1].toLowerCase()
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(value)
    ? value
    : `https://${value}`
  try {
    return new URL(withScheme).hostname.toLowerCase() || null
  } catch {
    return null
  }
}

// gitHostIconName picks the brand mark for a known public git host,
// null for anything else (including self-hosted instances).
export function gitHostIconName(repoUrl: string): BrandIconName | null {
  const host = gitHostname(repoUrl)?.replace(/^www\./, '')
  return host ? (GIT_HOST_ICONS[host] ?? null) : null
}
