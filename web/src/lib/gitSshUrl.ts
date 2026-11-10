// Matches the backend's SSH URL notion: ssh:// or scp-like user@host:path.
export function isSshRepoUrl(url: string): boolean {
  const u = url.trim()
  if (u.startsWith('ssh://')) return true
  if (u.includes('://')) return false
  return /^[^\s@/]+@[^\s:/]+:\S+/.test(u)
}
