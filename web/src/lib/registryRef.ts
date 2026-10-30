import type { BrandIconName } from '../components/BrandIcon'

// Registries whose address alone says "this is an image", so typing it needs no prefix.
const KNOWN_HOST =
  /^(?:docker\.io|index\.docker\.io|registry-1\.docker\.io|ghcr\.io|quay\.io|gcr\.io|mcr\.microsoft\.com|registry\.gitlab\.com|public\.ecr\.aws|registry\.k8s\.io|[a-z0-9-]+\.pkg\.dev|[a-z0-9-]+\.azurecr\.io|[0-9]+\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com)$/i
const REF =
  /^([a-z0-9-]+(?:\.[a-z0-9-]+)+|localhost)(?::\d+)?\/[A-Za-z0-9._/:@-]+$/i

/** The registry host of an image reference such as ghcr.io/org/app:1, or null. */
export function registryHostOf(ref: string): string | null {
  const m = REF.exec(ref.trim())
  return m?.[1]?.toLowerCase() ?? null
}

/** True for a reference on a well-known registry, which the palette treats as an image without a prefix. */
export function isKnownRegistryRef(ref: string): boolean {
  const host = registryHostOf(ref)
  return host !== null && KNOWN_HOST.test(host)
}

/** The brand mark that fits a registry host, or null when there is no mark. */
export function brandForRegistryHost(host: string): BrandIconName | null {
  const h = host.toLowerCase()
  if (/(^|\.)docker\.io$/.test(h)) return 'docker'
  if (h === 'ghcr.io') return 'github'
  if (h.includes('gitlab')) return 'gitlab'
  if (h === 'quay.io') return 'quay'
  if (h.endsWith('.azurecr.io')) return 'azure-container-registry'
  if (h === 'public.ecr.aws' || h.includes('.ecr.')) return 'ecr'
  if (h.includes('harbor')) return 'harbor'
  return null
}
