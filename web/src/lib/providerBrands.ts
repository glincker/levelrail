import type { BackupProvider } from '../types/backupTarget'
import type { StoragePreset } from '../types/storage'
import { endpointHostname, isHostOrSubdomain } from './brandLogos'

// Every function returns a BrandIcon key, or '' when no mark exists so
// BrandTile renders the provider initial instead.

const DNS_PROVIDER_BRAND: Record<string, string> = {
  cloudflare: 'cloudflare',
  route53: 'route53',
}

export function dnsProviderBrand(provider: string): string {
  return DNS_PROVIDER_BRAND[provider] ?? ''
}

const NODE_PROVIDER_BRAND: Record<string, string> = {
  hetzner: 'hetzner',
  digitalocean: 'digitalocean',
  aws: 'aws',
  azure: 'azure',
  gcp: 'google-cloud',
  linode: 'linode',
  akamai: 'akamai',
  oracle: 'oracle',
}

export function nodeProviderBrand(provider: string): string {
  return NODE_PROVIDER_BRAND[provider.toLowerCase()] ?? ''
}

const REGISTRY_HOST_BRANDS: Array<[(host: string) => boolean, string]> = [
  [
    (h) =>
      isHostOrSubdomain(h, 'docker.io') || isHostOrSubdomain(h, 'docker.com'),
    'docker',
  ],
  [(h) => isHostOrSubdomain(h, 'ghcr.io'), 'github'],
  [(h) => isHostOrSubdomain(h, 'gitlab.com'), 'gitlab'],
  [(h) => isHostOrSubdomain(h, 'registry.digitalocean.com'), 'digitalocean'],
  [
    (h) => isHostOrSubdomain(h, 'gcr.io') || isHostOrSubdomain(h, 'pkg.dev'),
    'google-artifact-registry',
  ],
  [(h) => isHostOrSubdomain(h, 'azurecr.io'), 'azure-container-registry'],
  [(h) => isHostOrSubdomain(h, 'quay.io'), 'quay'],
  [(h) => h.includes('.ecr.') && h.endsWith('.amazonaws.com'), 'ecr'],
  [(h) => isHostOrSubdomain(h, 'jfrog.io'), 'jfrog'],
  [(h) => h.split('.').includes('harbor'), 'harbor'],
]

export function registryBrandForHost(registryHost: string): string {
  const host = endpointHostname(registryHost)
  if (!host) return ''
  return REGISTRY_HOST_BRANDS.find(([match]) => match(host))?.[1] ?? ''
}

export function backupTargetBrand(
  provider: BackupProvider,
  endpoint?: string,
): string {
  if (provider === 'aws') return 's3'
  if (provider === 'r2') return 'cloudflare'
  const host = endpointHostname(endpoint)
  if (isHostOrSubdomain(host, 'backblazeb2.com')) return 'backblaze'
  if (isHostOrSubdomain(host, 'googleapis.com')) return 'google-cloud'
  if (host.split('.').some((label) => label.includes('minio'))) return 'minio'
  if (isHostOrSubdomain(host, 'r2.cloudflarestorage.com')) return 'cloudflare'
  if (isHostOrSubdomain(host, 'amazonaws.com')) return 's3'
  return ''
}

const STORAGE_PRESET_BRAND: Record<StoragePreset, string> = {
  aws: 's3',
  r2: 'cloudflare',
  b2: 'backblaze',
  minio: 'minio',
  wasabi: '',
  gcs: 'google-cloud-storage',
  custom: '',
}

export function storagePresetBrand(preset: StoragePreset): string {
  return STORAGE_PRESET_BRAND[preset]
}

const AI_BRAND_ALIAS: Record<string, string> = {
  gemini: 'google-gemini',
  'mistral-ai': 'mistral',
  together: 'together-ai',
  'x-ai': 'xai',
  grok: 'xai',
  'fireworks-ai': 'fireworks',
  'microsoft-azure': 'azure',
  claude: 'anthropic',
}

export function aiProviderBrand(brand: string | undefined): string {
  const key = (brand ?? '').trim().toLowerCase()
  return AI_BRAND_ALIAS[key] ?? key
}
