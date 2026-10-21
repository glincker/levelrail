import {
  Suspense,
  createElement,
  lazy,
  type ComponentType,
  type SVGProps,
} from 'react'

// Brand/service marks sourced from @thesvg/react, each its own lazy chunk so
// a page only downloads the marks it renders. Distinct from @phosphor-icons
// (ADR 014): brand marks identify a third-party technology, UI chrome icons
// don't.
export type BrandIconName =
  | 'docker'
  | 'postgres'
  | 'redis'
  | 'mysql'
  | 'mongodb'
  | 'mariadb'
  | 'keydb'
  | 'dragonfly'
  | 'valkey'
  | 'timescaledb'
  | 'clickhouse'
  | 'meilisearch'
  | 'qdrant'
  | 'typesense'
  | 'memcached'
  | 'node'
  | 'go'
  | 'java'
  | 'nextjs'
  | 'vite'
  | 'svelte'
  | 'nuxt'
  | 'astro'
  | 'django'
  | 'fastapi'
  | 'laravel'
  | 'spring'
  | 'ruby'
  | 'discord'
  | 'slack'
  | 'telegram'
  | 'pagerduty'
  | 'microsoft-teams'
  | 'github'
  | 'gitlab'
  | 'bitbucket'
  | 'mattermost'
  | 'ntfy'
  | 'resend'
  | 'rocketchat'
  | 'opsgenie'
  | 'webex'
  | 'google-chat'
  | 'google'
  | 'microsoft'
  | 'gitea'
  | 'forgejo'
  | 'openid'
  | 'facebook'
  | 'linkedin'
  | 'twitch'
  | 'linear'
  | 'vercel'
  | 'huggingface'
  | 'figma'
  | 'salesforce'
  | 'codeberg'
  | 'cognito'
  | 'dropbox'
  | 'kakao'
  | 'patreon'
  | 'okta'
  | 'keycloak'
  | 'auth0'
  | 'authentik'
  | 'cloudflare'
  | 'aws'
  | 'digitalocean'
  | 'hetzner'
  | 'linode'
  | 'akamai'
  | 'azure'
  | 'google-cloud'
  | 'oracle'
  | 'backblaze'
  | 'minio'
  | 'harbor'
  | 'jfrog'
  | 'route53'
  | 's3'
  | 'ecr'
  | 'google-artifact-registry'
  | 'gandi'
  | 'godaddy'
  | 'namecheap'
  | 'mailgun'
  | 'wasabi'
  | 'scaleway'
  | 'ovhcloud'
  | 'vultr'
  | 'google-cloud-storage'
  | 'quay'
  | 'azure-container-registry'
  | 'npm'
  | 'pypi'
  | 'maven'
  | 'nuget'
  | 'cargo'
  | 'rubygems'
  | 'newrelic'
  | 'datadog'
  | 'axiom'
  | 'betterstack'
  | 'grafana'
  | 'opentelemetry'
  | 'chrome'
  | 'firefox'
  | 'safari'
  | 'edge'
  | 'opera'
  | 'brave'
  | 'apple'
  | 'windows'
  | 'linux'
  | 'android'
  | 'curl'
  | 'openai'
  | 'anthropic'
  | 'google-gemini'
  | 'mistral'
  | 'groq'
  | 'together-ai'
  | 'openrouter'
  | 'deepseek'
  | 'xai'
  | 'fireworks'
  | 'nginx'
  | 'traefik'
  | 'caddy'
  | 'ollama'
  | 'perplexity'
  | 'vllm'
  | 'lm-studio'

type LogoComponent = ComponentType<SVGProps<SVGSVGElement>>
type VariantLogo = ComponentType<SVGProps<SVGSVGElement> & { variant?: string }>

const LOADERS: Record<
  BrandIconName,
  () => Promise<{ default: LogoComponent }>
> = {
  docker: () => import('@thesvg/react/docker'),
  postgres: () => import('@thesvg/react/postgresql'),
  redis: () => import('@thesvg/react/redis'),
  mysql: () => import('@thesvg/react/mysql'),
  mongodb: () => import('@thesvg/react/mongodb'),
  mariadb: () => import('@thesvg/react/mariadb'),
  keydb: () => import('@thesvg/react/keydb'),
  dragonfly: () => import('@thesvg/react/dragonfly'),
  valkey: () => import('@thesvg/react/valkey'),
  timescaledb: () => import('@thesvg/react/timescale'),
  clickhouse: () => import('@thesvg/react/clickhouse'),
  meilisearch: () => import('@thesvg/react/meilisearch'),
  qdrant: () => import('@thesvg/react/qdrant'),
  typesense: () => import('@thesvg/react/typesense'),
  memcached: () => import('@thesvg/react/memcached'),
  node: () => import('@thesvg/react/nodedotjs'),
  go: () => import('@thesvg/react/go'),
  java: () => import('@thesvg/react/java'),
  nextjs: () => import('@thesvg/react/nextjs'),
  vite: () => import('@thesvg/react/vite'),
  svelte: () => import('@thesvg/react/svelte'),
  nuxt: () => import('@thesvg/react/nuxt'),
  astro: () => import('@thesvg/react/astro'),
  django: () => import('@thesvg/react/django'),
  fastapi: () => import('@thesvg/react/fastapi'),
  laravel: () => import('@thesvg/react/laravel'),
  spring: () => import('@thesvg/react/spring'),
  ruby: () => import('@thesvg/react/ruby'),
  discord: () => import('@thesvg/react/discord'),
  slack: () => import('@thesvg/react/slack'),
  telegram: () => import('@thesvg/react/telegram'),
  pagerduty: () => import('@thesvg/react/pagerduty'),
  'microsoft-teams': () => import('@thesvg/react/microsoft-teams'),
  github: () => import('@thesvg/react/github'),
  gitlab: () => import('@thesvg/react/gitlab'),
  bitbucket: () => import('@thesvg/react/bitbucket'),
  mattermost: () => import('@thesvg/react/mattermost'),
  ntfy: () => import('@thesvg/react/ntfy'),
  resend: () => import('@thesvg/react/resend'),
  rocketchat: () => import('@thesvg/react/rocketdotchat'),
  opsgenie: () => import('@thesvg/react/opsgenie'),
  webex: () => import('@thesvg/react/webex'),
  'google-chat': () => import('@thesvg/react/google-chat'),
  google: () => import('@thesvg/react/google'),
  microsoft: () => import('@thesvg/react/microsoft'),
  gitea: () => import('@thesvg/react/gitea'),
  forgejo: () => import('@thesvg/react/forgejo'),
  openid: () => import('@thesvg/react/openid'),
  facebook: () => import('@thesvg/react/facebook'),
  linkedin: () => import('@thesvg/react/linkedin'),
  twitch: () => import('@thesvg/react/twitch'),
  linear: () => import('@thesvg/react/linear'),
  vercel: () => import('@thesvg/react/vercel'),
  huggingface: () => import('@thesvg/react/hugging-face'),
  figma: () => import('@thesvg/react/figma'),
  salesforce: () => import('@thesvg/react/salesforce'),
  codeberg: () => import('@thesvg/react/codeberg'),
  cognito: () => import('@thesvg/react/aws-amazon-cognito'),
  dropbox: () => import('@thesvg/react/dropbox'),
  kakao: () => import('@thesvg/react/kakao'),
  patreon: () => import('@thesvg/react/patreon'),
  okta: () => import('@thesvg/react/okta'),
  keycloak: () => import('@thesvg/react/keycloak'),
  auth0: () => import('@thesvg/react/auth0'),
  authentik: () => import('@thesvg/react/authentik'),
  cloudflare: () => import('@thesvg/react/cloudflare'),
  aws: () => import('@thesvg/react/aws'),
  digitalocean: () => import('@thesvg/react/digitalocean'),
  hetzner: () => import('@thesvg/react/hetzner'),
  linode: () => import('@thesvg/react/linode'),
  akamai: () => import('@thesvg/react/akamai'),
  azure: () => import('@thesvg/react/microsoft-azure'),
  'google-cloud': () => import('@thesvg/react/google-cloud'),
  oracle: () => import('@thesvg/react/oracle'),
  backblaze: () => import('@thesvg/react/backblaze'),
  minio: () => import('@thesvg/react/minio'),
  harbor: () => import('@thesvg/react/harbor'),
  jfrog: () => import('@thesvg/react/jfrog'),
  route53: () => import('@thesvg/react/aws-amazon-route-53'),
  s3: () => import('@thesvg/react/aws-amazon-simple-storage-service'),
  ecr: () => import('@thesvg/react/aws-amazon-elastic-container-registry'),
  'google-artifact-registry': () =>
    import('@thesvg/react/gcp-artifact-registry'),
  gandi: () => import('@thesvg/react/gandi'),
  godaddy: () => import('@thesvg/react/godaddy'),
  namecheap: () => import('@thesvg/react/namecheap'),
  mailgun: () => import('@thesvg/react/mailgun'),
  wasabi: () => import('@thesvg/react/wasabi'),
  scaleway: () => import('@thesvg/react/scaleway'),
  ovhcloud: () => import('@thesvg/react/ovhcloud'),
  vultr: () => import('@thesvg/react/vultr'),
  'google-cloud-storage': () => import('@thesvg/react/google-cloud-storage'),
  quay: () => import('@thesvg/react/quay'),
  'azure-container-registry': () =>
    import('@thesvg/react/azure-container-registries'),
  npm: () => import('@thesvg/react/npm'),
  pypi: () => import('@thesvg/react/pypi'),
  maven: () => import('@thesvg/react/apache-maven'),
  nuget: () => import('@thesvg/react/nuget'),
  cargo: () => import('@thesvg/react/rust'),
  rubygems: () => import('@thesvg/react/rubygems'),
  newrelic: () => import('@thesvg/react/new-relic'),
  datadog: () => import('@thesvg/react/datadog'),
  axiom: () => import('@thesvg/react/axiom'),
  betterstack: () => import('@thesvg/react/better-stack'),
  grafana: () => import('@thesvg/react/grafana'),
  opentelemetry: () => import('@thesvg/react/opentelemetry'),
  chrome: () => import('@thesvg/react/google-chrome'),
  firefox: () => import('@thesvg/react/firefox'),
  safari: () => import('@thesvg/react/safari'),
  edge: () => import('@thesvg/react/microsoft-edge'),
  opera: () => import('@thesvg/react/opera'),
  brave: () => import('@thesvg/react/brave'),
  apple: () => import('@thesvg/react/apple'),
  windows: () => import('@thesvg/react/windows'),
  linux: () => import('@thesvg/react/linux'),
  android: () => import('@thesvg/react/android'),
  curl: () => import('@thesvg/react/curl'),
  openai: () => import('@thesvg/react/openai'),
  anthropic: () => import('@thesvg/react/claude'),
  'google-gemini': () => import('@thesvg/react/google-gemini'),
  mistral: () => import('@thesvg/react/mistral'),
  groq: () => import('@thesvg/react/groq'),
  'together-ai': () => import('@thesvg/react/together-ai'),
  openrouter: () => import('@thesvg/react/openrouter'),
  deepseek: () => import('@thesvg/react/deepseek'),
  xai: () => import('@thesvg/react/xai'),
  fireworks: () => import('@thesvg/react/fireworks'),
  nginx: () => import('@thesvg/react/nginx'),
  traefik: () => import('@thesvg/react/traefik'),
  caddy: () => import('@thesvg/react/caddy'),
  ollama: () => import('@thesvg/react/ollama'),
  perplexity: () => import('@thesvg/react/perplexity'),
  vllm: () => import('@thesvg/react/vllm'),
  'lm-studio': () => import('@thesvg/react/lm-studio'),
}

// All names in LOADERS, for callers that enumerate the available marks (a
// picker grid) without listing them a second time.
// eslint-disable-next-line react-refresh/only-export-components
export const BRAND_ICON_NAMES = Object.keys(LOADERS) as BrandIconName[]

// Default variants that don't work on a light tile (white-only or wordmark lockups).
const DEFAULT_VARIANT: Partial<Record<BrandIconName, string>> = {
  mysql: 'light',
  go: 'mono',
  cargo: 'light',
  mattermost: 'light',
  openrouter: 'light',
}

// Brands with a single-colour variant that follows currentColor.
const MONO_NAMES: ReadonlySet<BrandIconName> = new Set<BrandIconName>([
  'github',
  'google',
  'gitea',
  'go',
  'cloudflare',
  'docker',
  'aws',
  'digitalocean',
  'hetzner',
  'akamai',
  'google-cloud',
  'backblaze',
  'minio',
  'harbor',
  'jfrog',
])

const NAME_SET: ReadonlySet<string> = new Set(BRAND_ICON_NAMES)

// eslint-disable-next-line react-refresh/only-export-components
export function isBrandIconName(name: string): name is BrandIconName {
  return NAME_SET.has(name)
}

// eslint-disable-next-line react-refresh/only-export-components
export function hasMonoVariant(name: BrandIconName): boolean {
  return MONO_NAMES.has(name)
}

function lazyLogo(name: BrandIconName, variant: string | undefined) {
  return lazy(async () => {
    const Logo = (await LOADERS[name]()).default as VariantLogo
    const Variant: LogoComponent = (props) =>
      createElement(Logo, variant ? { ...props, variant } : props)
    return { default: Variant }
  })
}

const DEFAULT_LOGOS = {} as Record<BrandIconName, LogoComponent>
const MONO_LOGOS: Partial<Record<BrandIconName, LogoComponent>> = {}
for (const n of BRAND_ICON_NAMES) {
  DEFAULT_LOGOS[n] = lazyLogo(n, DEFAULT_VARIANT[n])
  if (MONO_NAMES.has(n)) MONO_LOGOS[n] = lazyLogo(n, 'mono')
}

export interface BrandIconProps extends SVGProps<SVGSVGElement> {
  /** Which brand mark to render. */
  name: BrandIconName
  /** Single-colour mark that follows currentColor, when the brand has one. */
  mono?: boolean
}

// Renders a single brand/service logo. Size via a Tailwind `size-*`
// class on `className`; `aria-hidden="true"` by default since the mark
// is normally paired with a visible text label.
export function BrandIcon({
  name,
  mono = false,
  'aria-hidden': ariaHidden,
  ...props
}: BrandIconProps) {
  const Logo = (mono ? MONO_LOGOS[name] : undefined) ?? DEFAULT_LOGOS[name]
  return (
    <Suspense
      fallback={
        <span
          aria-hidden="true"
          className={`inline-block ${props.className ?? ''}`}
        />
      }
    >
      <Logo aria-hidden={ariaHidden ?? 'true'} {...props} />
    </Suspense>
  )
}
