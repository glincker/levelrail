export interface ImageRegistryLink {
  registry: string
  url: string
}

function stripTagAndDigest(image: string): string {
  const noDigest = image.split('@')[0] ?? image
  const slash = noDigest.lastIndexOf('/')
  const colon = noDigest.lastIndexOf(':')
  return colon > slash ? noDigest.slice(0, colon) : noDigest
}

function hubLink(path: string): ImageRegistryLink {
  const url = path.includes('/')
    ? `https://hub.docker.com/r/${path}`
    : `https://hub.docker.com/_/${path}`
  return { registry: 'Docker Hub', url }
}

// Maps an image reference to its registry page. Returns null when the
// registry has no stable, derivable page URL (private or self-hosted hosts).
export function registryLinkForImage(image: string): ImageRegistryLink | null {
  const ref = stripTagAndDigest(image.trim())
  if (ref === '' || ref.includes('$')) return null
  const parts = ref.split('/')
  const first = parts[0] ?? ''
  const hasHost = parts.length > 1 && /[.:]|^localhost$/.test(first)
  if (!hasHost) {
    return hubLink(first === 'library' ? parts.slice(1).join('/') : ref)
  }
  const rest = parts.slice(1)
  if (first === 'docker.io' || first === 'index.docker.io') {
    const path = rest.join('/')
    return hubLink(path.startsWith('library/') ? path.slice(8) : path)
  }
  if (first === 'ghcr.io' && rest.length >= 2) {
    const [owner, ...name] = rest
    return {
      registry: 'GitHub Container Registry',
      url: `https://github.com/${String(owner)}?tab=packages&q=${encodeURIComponent(name.join('/'))}`,
    }
  }
  if (first === 'quay.io' && rest.length >= 2) {
    return {
      registry: 'Quay',
      url: `https://quay.io/repository/${rest.join('/')}`,
    }
  }
  if (first === 'lscr.io' && rest[0] === 'linuxserver' && rest.length === 2) {
    return hubLink(rest.join('/'))
  }
  return null
}
