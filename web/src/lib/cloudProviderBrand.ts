import { nodeProviderBrand } from './providerBrands'

// Sections the landing page can render. A later slice (discovery, spend,
// firewalls, volumes, DOKS) adds its id here, registers a component in
// components/cloudProvider/sections.tsx and lists the id per provider below.
export type CloudProviderFeature =
  'connection' | 'capabilities' | 'servers' | 'create' | 'resources' | 'spend'

export const CLOUD_PROVIDER_IDS = [
  'digitalocean',
  'hetzner',
  'vultr',
  'aws',
  'azure',
  'gcp',
  'oracle',
  'custom',
] as const

export type CloudProviderId = (typeof CLOUD_PROVIDER_IDS)[number]

export interface CloudProviderPresentation {
  id: CloudProviderId
  /** thesvg brand key, empty when the provider has no mark. */
  brand: string
  /** Tailwind classes, literal so the compiler keeps them. Empty means no accent. */
  glowClass: string
  chipClass: string
  consoleUrl: string
  tokenUrl: string
  docsUrl: string
  features: readonly CloudProviderFeature[]
  /** Whether the regions API can be used for the quick facts row. */
  quickFactRegions: boolean
}

const BASE: readonly CloudProviderFeature[] = [
  'connection',
  'capabilities',
  'servers',
  'create',
]

// Adding a provider is one entry here plus its keys under providers.<id> in
// locales/*/cloudProvider.json (name, tagline, terms, create, tokenHelp).
export const CLOUD_PROVIDERS: Record<
  CloudProviderId,
  CloudProviderPresentation
> = {
  digitalocean: {
    id: 'digitalocean',
    brand: nodeProviderBrand('digitalocean'),
    glowClass: 'from-sky-500/15',
    chipClass: 'border-sky-500/40 bg-sky-500/10',
    consoleUrl: 'https://cloud.digitalocean.com/',
    tokenUrl: 'https://cloud.digitalocean.com/account/api/tokens',
    docsUrl:
      'https://docs.digitalocean.com/reference/api/create-personal-access-token/',
    features: [...BASE, 'resources'],
    quickFactRegions: true,
  },
  hetzner: {
    id: 'hetzner',
    brand: nodeProviderBrand('hetzner'),
    glowClass: 'from-red-500/15',
    chipClass: 'border-red-500/40 bg-red-500/10',
    consoleUrl: 'https://console.hetzner.cloud/',
    tokenUrl: 'https://console.hetzner.cloud/',
    docsUrl:
      'https://docs.hetzner.com/cloud/api/getting-started/generating-api-token/',
    features: BASE,
    quickFactRegions: true,
  },
  vultr: {
    id: 'vultr',
    brand: nodeProviderBrand('vultr'),
    glowClass: 'from-blue-500/15',
    chipClass: 'border-blue-500/40 bg-blue-500/10',
    consoleUrl: 'https://my.vultr.com/',
    tokenUrl: 'https://my.vultr.com/settings/#settingsapi',
    docsUrl: 'https://docs.vultr.com/vultr-api',
    features: BASE,
    quickFactRegions: true,
  },
  aws: {
    id: 'aws',
    brand: nodeProviderBrand('aws'),
    glowClass: 'from-amber-500/15',
    chipClass: 'border-amber-500/40 bg-amber-500/10',
    consoleUrl: 'https://console.aws.amazon.com/',
    tokenUrl: 'https://console.aws.amazon.com/iam/home#/security_credentials',
    docsUrl:
      'https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html',
    features: BASE,
    quickFactRegions: true,
  },
  azure: {
    id: 'azure',
    brand: nodeProviderBrand('azure'),
    glowClass: 'from-blue-500/15',
    chipClass: 'border-blue-500/40 bg-blue-500/10',
    consoleUrl: 'https://portal.azure.com/',
    tokenUrl:
      'https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade',
    docsUrl:
      'https://learn.microsoft.com/entra/identity-platform/howto-create-service-principal-portal',
    features: BASE,
    quickFactRegions: true,
  },
  gcp: {
    id: 'gcp',
    brand: nodeProviderBrand('gcp'),
    glowClass: 'from-green-500/15',
    chipClass: 'border-green-500/40 bg-green-500/10',
    consoleUrl: 'https://console.cloud.google.com/',
    tokenUrl: 'https://console.cloud.google.com/iam-admin/serviceaccounts',
    docsUrl: 'https://cloud.google.com/iam/docs/keys-create-delete',
    features: BASE,
    quickFactRegions: true,
  },
  oracle: {
    id: 'oracle',
    brand: nodeProviderBrand('oracle'),
    glowClass: 'from-rose-500/15',
    chipClass: 'border-rose-500/40 bg-rose-500/10',
    consoleUrl: 'https://cloud.oracle.com/',
    tokenUrl: 'https://cloud.oracle.com/identity/domains/my-profile/api-keys',
    docsUrl:
      'https://docs.oracle.com/en-us/iaas/Content/API/Concepts/apisigningkey.htm',
    features: BASE,
    quickFactRegions: true,
  },
  // A manual data center has no cloud API, so no capability check or console.
  custom: {
    id: 'custom',
    brand: '',
    glowClass: '',
    chipClass: 'border-border bg-muted/40',
    consoleUrl: '',
    tokenUrl: '',
    docsUrl: '',
    features: ['connection', 'servers', 'create'],
    quickFactRegions: false,
  },
}

export function cloudProviderFor(id: string): CloudProviderPresentation | null {
  return (CLOUD_PROVIDER_IDS as readonly string[]).includes(id)
    ? CLOUD_PROVIDERS[id as CloudProviderId]
    : null
}

export function cloudProviderPath(id: string): string {
  return `/integrations/cloud/${encodeURIComponent(id)}`
}
