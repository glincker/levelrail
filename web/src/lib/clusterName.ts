// Suggests a cluster name from the API host so the common case needs no typing.
export function suggestClusterName(server: string): string {
  try {
    const host = new URL(server).hostname
    return host
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 63)
  } catch {
    return ''
  }
}
