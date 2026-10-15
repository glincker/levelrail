import type { ComponentType, SVGProps } from 'react'
import BitbucketLogo from '@thesvg/react/bitbucket'
import Auth0Logo from '@thesvg/react/auth0'
import AuthentikLogo from '@thesvg/react/authentik'
import ClickhouseLogo from '@thesvg/react/clickhouse'
import CloudflareLogo from '@thesvg/react/cloudflare'
import DiscordLogo from '@thesvg/react/discord'
import DockerLogo from '@thesvg/react/docker'
import DragonflyLogo from '@thesvg/react/dragonfly'
import CodebergLogo from '@thesvg/react/codeberg'
import CognitoLogo from '@thesvg/react/aws-amazon-cognito'
import ValkeyLogo from '@thesvg/react/valkey'
import NewRelicLogo from '@thesvg/react/new-relic'
import DatadogLogo from '@thesvg/react/datadog'
import Axiom2Logo from '@thesvg/react/axiom'
import BetterStackLogo from '@thesvg/react/better-stack'
import GrafanaLogo from '@thesvg/react/grafana'
import OpenTelemetryLogo from '@thesvg/react/opentelemetry'
import ChromeLogo from '@thesvg/react/google-chrome'
import FirefoxLogo from '@thesvg/react/firefox'
import SafariLogo from '@thesvg/react/safari'
import EdgeLogo from '@thesvg/react/microsoft-edge'
import OperaLogo from '@thesvg/react/opera'
import BraveLogo from '@thesvg/react/brave'
import AppleLogo from '@thesvg/react/apple'
import WindowsLogo from '@thesvg/react/windows'
import LinuxLogo from '@thesvg/react/linux'
import AndroidLogo from '@thesvg/react/android'
import CurlLogo from '@thesvg/react/curl'
import MeilisearchLogo from '@thesvg/react/meilisearch'
import QdrantLogo from '@thesvg/react/qdrant'
import TypesenseLogo from '@thesvg/react/typesense'
import MemcachedLogo from '@thesvg/react/memcached'
import TimescaleLogo from '@thesvg/react/timescale'
import FacebookLogo from '@thesvg/react/facebook'
import FigmaLogo from '@thesvg/react/figma'
import HuggingFaceLogo from '@thesvg/react/hugging-face'
import LinearLogo from '@thesvg/react/linear'
import SalesforceLogo from '@thesvg/react/salesforce'
import VercelLogo from '@thesvg/react/vercel'
import GiteaLogo from '@thesvg/react/gitea'
import GitHubLogo from '@thesvg/react/github'
import GitLabLogo from '@thesvg/react/gitlab'
import GoLogo from '@thesvg/react/go'
import GoogleLogo from '@thesvg/react/google'
import GoogleChatLogo from '@thesvg/react/google-chat'
import JavaLogo from '@thesvg/react/java'
import KeycloakLogo from '@thesvg/react/keycloak'
import KeydbLogo from '@thesvg/react/keydb'
import MariadbLogo from '@thesvg/react/mariadb'
import MattermostLogo from '@thesvg/react/mattermost'
import LinkedInLogo from '@thesvg/react/linkedin'
import MicrosoftLogo from '@thesvg/react/microsoft'
import MicrosoftTeamsLogo from '@thesvg/react/microsoft-teams'
import MongodbLogo from '@thesvg/react/mongodb'
import MysqlLogo from '@thesvg/react/mysql'
import NodedotjsLogo from '@thesvg/react/nodedotjs'
import NtfyLogo from '@thesvg/react/ntfy'
import OktaLogo from '@thesvg/react/okta'
import OpenidLogo from '@thesvg/react/openid'
import OpsgenieLogo from '@thesvg/react/opsgenie'
import PagerdutyLogo from '@thesvg/react/pagerduty'
import PostgresqlLogo from '@thesvg/react/postgresql'
import RedisLogo from '@thesvg/react/redis'
import ResendLogo from '@thesvg/react/resend'
import RocketdotchatLogo from '@thesvg/react/rocketdotchat'
import SlackLogo from '@thesvg/react/slack'
import TwitchLogo from '@thesvg/react/twitch'
import TelegramLogo from '@thesvg/react/telegram'
import WebexLogo from '@thesvg/react/webex'
import AwsLogo from '@thesvg/react/aws'
import DigitaloceanLogo from '@thesvg/react/digitalocean'
import HetznerLogo from '@thesvg/react/hetzner'
import LinodeLogo from '@thesvg/react/linode'
import AkamaiLogo from '@thesvg/react/akamai'
import AzureLogo from '@thesvg/react/microsoft-azure'
import GoogleCloudLogo from '@thesvg/react/google-cloud'
import OracleLogo from '@thesvg/react/oracle'
import BackblazeLogo from '@thesvg/react/backblaze'
import MinioLogo from '@thesvg/react/minio'
import HarborLogo from '@thesvg/react/harbor'
import JfrogLogo from '@thesvg/react/jfrog'
import Route53Logo from '@thesvg/react/aws-amazon-route-53'
import S3Logo from '@thesvg/react/aws-amazon-simple-storage-service'
import EcrLogo from '@thesvg/react/aws-amazon-elastic-container-registry'
import ArtifactRegistryLogo from '@thesvg/react/gcp-artifact-registry'
import GandiLogo from '@thesvg/react/gandi'
import GodaddyLogo from '@thesvg/react/godaddy'
import NamecheapLogo from '@thesvg/react/namecheap'
import MailgunLogo from '@thesvg/react/mailgun'
import WasabiLogo from '@thesvg/react/wasabi'
import ScalewayLogo from '@thesvg/react/scaleway'
import OvhcloudLogo from '@thesvg/react/ovhcloud'
import VultrLogo from '@thesvg/react/vultr'
import GoogleCloudStorageLogo from '@thesvg/react/google-cloud-storage'
import QuayLogo from '@thesvg/react/quay'
import AzureContainerRegistryLogo from '@thesvg/react/azure-container-registries'
import NpmLogo from '@thesvg/react/npm'
import PypiLogo from '@thesvg/react/pypi'
import MavenLogo from '@thesvg/react/apache-maven'
import NugetLogo from '@thesvg/react/nuget'
import CargoLogo from '@thesvg/react/rust'
import RubygemsLogo from '@thesvg/react/rubygems'

// Brand/service marks sourced from @thesvg/react, each imported from its
// own subpath for bundle size. Distinct from @phosphor-icons (ADR 014):
// brand marks identify a third-party technology, UI chrome icons don't.
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

const BRAND_ICONS: Record<
  BrandIconName,
  ComponentType<SVGProps<SVGSVGElement>>
> = {
  docker: DockerLogo,
  postgres: PostgresqlLogo,
  redis: RedisLogo,
  mysql: MysqlLogo,
  mongodb: MongodbLogo,
  mariadb: MariadbLogo,
  keydb: KeydbLogo,
  dragonfly: DragonflyLogo,
  valkey: ValkeyLogo,
  timescaledb: TimescaleLogo,
  clickhouse: ClickhouseLogo,
  meilisearch: MeilisearchLogo,
  qdrant: QdrantLogo,
  typesense: TypesenseLogo,
  memcached: MemcachedLogo,
  node: NodedotjsLogo,
  go: GoLogo,
  java: JavaLogo,
  discord: DiscordLogo,
  slack: SlackLogo,
  telegram: TelegramLogo,
  pagerduty: PagerdutyLogo,
  'microsoft-teams': MicrosoftTeamsLogo,
  github: GitHubLogo,
  gitlab: GitLabLogo,
  bitbucket: BitbucketLogo,
  mattermost: MattermostLogo,
  ntfy: NtfyLogo,
  resend: ResendLogo,
  rocketchat: RocketdotchatLogo,
  opsgenie: OpsgenieLogo,
  webex: WebexLogo,
  'google-chat': GoogleChatLogo,
  google: GoogleLogo,
  microsoft: MicrosoftLogo,
  gitea: GiteaLogo,
  openid: OpenidLogo,
  facebook: FacebookLogo,
  linkedin: LinkedInLogo,
  twitch: TwitchLogo,
  linear: LinearLogo,
  vercel: VercelLogo,
  huggingface: HuggingFaceLogo,
  figma: FigmaLogo,
  salesforce: SalesforceLogo,
  codeberg: CodebergLogo,
  cognito: CognitoLogo,
  okta: OktaLogo,
  keycloak: KeycloakLogo,
  auth0: Auth0Logo,
  authentik: AuthentikLogo,
  cloudflare: CloudflareLogo,
  aws: AwsLogo,
  digitalocean: DigitaloceanLogo,
  hetzner: HetznerLogo,
  linode: LinodeLogo,
  akamai: AkamaiLogo,
  azure: AzureLogo,
  'google-cloud': GoogleCloudLogo,
  oracle: OracleLogo,
  backblaze: BackblazeLogo,
  minio: MinioLogo,
  harbor: HarborLogo,
  jfrog: JfrogLogo,
  route53: Route53Logo,
  s3: S3Logo,
  ecr: EcrLogo,
  'google-artifact-registry': ArtifactRegistryLogo,
  gandi: GandiLogo,
  godaddy: GodaddyLogo,
  namecheap: NamecheapLogo,
  mailgun: MailgunLogo,
  wasabi: WasabiLogo,
  scaleway: ScalewayLogo,
  ovhcloud: OvhcloudLogo,
  vultr: VultrLogo,
  'google-cloud-storage': GoogleCloudStorageLogo,
  quay: QuayLogo,
  'azure-container-registry': AzureContainerRegistryLogo,
  npm: NpmLogo,
  pypi: PypiLogo,
  maven: MavenLogo,
  nuget: NugetLogo,
  cargo: CargoLogo,
  rubygems: RubygemsLogo,
  newrelic: NewRelicLogo,
  datadog: DatadogLogo,
  axiom: Axiom2Logo,
  betterstack: BetterStackLogo,
  grafana: GrafanaLogo,
  opentelemetry: OpenTelemetryLogo,
  chrome: ChromeLogo,
  firefox: FirefoxLogo,
  safari: SafariLogo,
  edge: EdgeLogo,
  opera: OperaLogo,
  brave: BraveLogo,
  apple: AppleLogo,
  windows: WindowsLogo,
  linux: LinuxLogo,
  android: AndroidLogo,
  curl: CurlLogo,
}

// All names in BRAND_ICONS, for callers that need to enumerate the
// available brand marks (e.g. a picker grid) without hardcoding the list
// a second time.
// eslint-disable-next-line react-refresh/only-export-components
export const BRAND_ICON_NAMES: readonly BrandIconName[] = [
  'docker',
  'postgres',
  'redis',
  'mysql',
  'mongodb',
  'mariadb',
  'keydb',
  'dragonfly',
  'valkey',
  'timescaledb',
  'clickhouse',
  'meilisearch',
  'qdrant',
  'typesense',
  'memcached',
  'node',
  'go',
  'java',
  'discord',
  'slack',
  'telegram',
  'pagerduty',
  'microsoft-teams',
  'github',
  'gitlab',
  'bitbucket',
  'mattermost',
  'ntfy',
  'resend',
  'rocketchat',
  'opsgenie',
  'webex',
  'google-chat',
  'google',
  'microsoft',
  'gitea',
  'openid',
  'facebook',
  'linkedin',
  'twitch',
  'linear',
  'vercel',
  'huggingface',
  'figma',
  'salesforce',
  'codeberg',
  'cognito',
  'okta',
  'keycloak',
  'auth0',
  'authentik',
  'cloudflare',
  'aws',
  'digitalocean',
  'hetzner',
  'linode',
  'akamai',
  'azure',
  'google-cloud',
  'oracle',
  'backblaze',
  'minio',
  'harbor',
  'jfrog',
  'route53',
  's3',
  'ecr',
  'google-artifact-registry',
  'gandi',
  'godaddy',
  'namecheap',
  'mailgun',
  'wasabi',
  'scaleway',
  'ovhcloud',
  'vultr',
  'google-cloud-storage',
  'quay',
  'azure-container-registry',
  'npm',
  'pypi',
  'maven',
  'nuget',
  'cargo',
  'rubygems',
  'newrelic',
  'datadog',
  'axiom',
  'betterstack',
  'grafana',
  'opentelemetry',
  'chrome',
  'firefox',
  'safari',
  'edge',
  'opera',
  'brave',
  'apple',
  'windows',
  'linux',
  'android',
  'curl',
]

type SvgComponent = ComponentType<SVGProps<SVGSVGElement>>

// Single-colour variants that follow currentColor, for tiles that tint a
// mark (disconnected state, dark surfaces) instead of using brand colours.
const MONO_ICONS: Partial<Record<BrandIconName, SvgComponent>> = {
  github: (p) => <GitHubLogo variant="mono" {...p} />,
  google: (p) => <GoogleLogo variant="mono" {...p} />,
  gitea: (p) => <GiteaLogo variant="mono" {...p} />,
  go: (p) => <GoLogo variant="mono" {...p} />,
  cloudflare: (p) => <CloudflareLogo variant="mono" {...p} />,
  docker: (p) => <DockerLogo variant="mono" {...p} />,
  aws: (p) => <AwsLogo variant="mono" {...p} />,
  digitalocean: (p) => <DigitaloceanLogo variant="mono" {...p} />,
  hetzner: (p) => <HetznerLogo variant="mono" {...p} />,
  akamai: (p) => <AkamaiLogo variant="mono" {...p} />,
  'google-cloud': (p) => <GoogleCloudLogo variant="mono" {...p} />,
  backblaze: (p) => <BackblazeLogo variant="mono" {...p} />,
  minio: (p) => <MinioLogo variant="mono" {...p} />,
  harbor: (p) => <HarborLogo variant="mono" {...p} />,
  jfrog: (p) => <JfrogLogo variant="mono" {...p} />,
}

const NAME_SET: ReadonlySet<string> = new Set(BRAND_ICON_NAMES)

// eslint-disable-next-line react-refresh/only-export-components
export function isBrandIconName(name: string): name is BrandIconName {
  return NAME_SET.has(name)
}

// eslint-disable-next-line react-refresh/only-export-components
export function hasMonoVariant(name: BrandIconName): boolean {
  return name in MONO_ICONS
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
  // mysql/go's default variants don't work on a light background
  // (white-on-transparent; a wordmark lockup), so both use an explicit
  // non-default variant. Every other icon's default variant is fine.
  const MonoLogo = mono ? MONO_ICONS[name] : undefined
  if (MonoLogo) {
    return <MonoLogo aria-hidden={ariaHidden ?? 'true'} {...props} />
  }
  if (name === 'mysql') {
    return (
      <MysqlLogo
        variant="light"
        aria-hidden={ariaHidden ?? 'true'}
        {...props}
      />
    )
  }
  if (name === 'go') {
    return (
      <GoLogo variant="mono" aria-hidden={ariaHidden ?? 'true'} {...props} />
    )
  }
  // White-only default variants vanish on the white tile.
  if (name === 'cargo') {
    return (
      <CargoLogo
        variant="light"
        aria-hidden={ariaHidden ?? 'true'}
        {...props}
      />
    )
  }
  if (name === 'mattermost') {
    return (
      <MattermostLogo
        variant="light"
        aria-hidden={ariaHidden ?? 'true'}
        {...props}
      />
    )
  }
  const Logo = BRAND_ICONS[name]
  return <Logo aria-hidden={ariaHidden ?? 'true'} {...props} />
}
