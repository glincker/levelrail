import { isBrandIconName, type BrandIconName } from '../components/BrandIcon'
import type { ConnectionKind } from '../types/connections'
import { registryBrandForHost } from './providerBrands'

// Single source for which mark each integration provider shows: a brand mark
// from thesvg, or a semantic UI icon where the library has no brand.
export type IntegrationUiIcon = 'mail' | 'bell' | 'webhook' | 'server'

export type IntegrationGlyphSpec =
  | { brand: BrandIconName; ui?: undefined }
  | { ui: IntegrationUiIcon; brand?: undefined }

const b = (brand: BrandIconName): IntegrationGlyphSpec => ({ brand })
const u = (ui: IntegrationUiIcon): IntegrationGlyphSpec => ({ ui })

export const INTEGRATION_GLYPHS: Record<
  ConnectionKind,
  Record<string, IntegrationGlyphSpec>
> = {
  git: {
    github: b('github'),
    gitlab: b('gitlab'),
    bitbucket: b('bitbucket'),
    gitea: b('gitea'),
    forgejo: b('forgejo'),
    // No dedicated marks in thesvg: the vendor family mark is the closest honest one.
    bitbucket_server: b('bitbucket'),
    azure_devops: b('azure-devops'),
  },
  registry: {
    dockerhub: b('docker'),
    ghcr: b('github'),
    gitlab: b('gitlab'),
    ecr: b('ecr'),
    artifact_registry: b('google-artifact-registry'),
    gar: b('google-artifact-registry'),
    acr: b('azure-container-registry'),
    harbor: b('harbor'),
    quay: b('quay'),
    jfrog: b('jfrog'),
    nexus: b('sonatype'),
    generic: u('server'),
  },
  notify: {
    slack: b('slack'),
    discord: b('discord'),
    telegram: b('telegram'),
    email: u('mail'),
    pushover: b('pushover'),
    pagerduty: b('pagerduty'),
    teams: b('microsoft-teams'),
    resend: b('resend'),
    ntfy: b('ntfy'),
    gotify: b('gotify'),
    mattermost: b('mattermost'),
    lark: b('lark'),
    rocketchat: b('rocketchat'),
    opsgenie: b('opsgenie'),
    webex: b('webex'),
    googlechat: b('google-chat'),
    matrix: b('matrix-org'),
    zulip: b('zulip'),
    webpush: u('bell'),
    generic: u('webhook'),
  },
  secret: {
    vault: b('vault'),
    doppler: b('doppler'),
    infisical: b('infisical'),
    onepassword: b('onepassword'),
  },
  dns: {
    cloudflare: b('cloudflare'),
    route53: b('route53'),
    google: b('google-cloud'),
  },
  cloud: {
    oracle: b('oracle'),
    aws: b('aws'),
    azure: b('azure'),
    gcp: b('google-cloud'),
    hetzner: b('hetzner'),
    digitalocean: b('digitalocean'),
  },
  webhook: {
    event: u('webhook'),
    status: u('webhook'),
  },
}

// Null means "use the neutral kind icon". A registry provider can also be a bare host.
export function integrationGlyph(
  kind: string,
  provider: string,
): IntegrationGlyphSpec | null {
  const id = provider.toLowerCase()
  const byKind = INTEGRATION_GLYPHS[kind as ConnectionKind]
  const hit = byKind ? Object.hasOwn(byKind, id) && byKind[id] : undefined
  if (hit) return hit
  if (kind === 'registry') {
    const host = registryBrandForHost(id)
    if (isBrandIconName(host)) return b(host)
  }
  return null
}
