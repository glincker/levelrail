// Mirrors internal/k8sconn/pullsecret.go so the form can warn before the server answers 400.
const HUB = 'docker.io'
const HUB_ALIASES = new Set([
  'index.docker.io',
  'registry-1.docker.io',
  'registry.hub.docker.com',
])
const HOST_RE = /^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?(:[0-9]{1,5})?$/

export function normalizeRegistryHost(s: string): string {
  let h = s.trim().toLowerCase()
  h = h.replace(/^https?:\/\//, '')
  h = h.split('/')[0] ?? ''
  if (HUB_ALIASES.has(h)) h = HUB
  return HOST_RE.test(h) ? h : ''
}

export function imageRegistryHost(image: string): string {
  const i = image.indexOf('/')
  if (i < 0) return HUB
  const first = image.slice(0, i)
  if (/[.:]/.test(first) || first === 'localhost') {
    return normalizeRegistryHost(first)
  }
  return HUB
}
