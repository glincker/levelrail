import type { ComponentType, SVGProps } from 'react'
import ClickhouseLogo from '@thesvg/react/clickhouse'
import DiscordLogo from '@thesvg/react/discord'
import DockerLogo from '@thesvg/react/docker'
import DragonflyLogo from '@thesvg/react/dragonfly'
import GoLogo from '@thesvg/react/go'
import JavaLogo from '@thesvg/react/java'
import KeydbLogo from '@thesvg/react/keydb'
import MariadbLogo from '@thesvg/react/mariadb'
import MicrosoftTeamsLogo from '@thesvg/react/microsoft-teams'
import MongodbLogo from '@thesvg/react/mongodb'
import MysqlLogo from '@thesvg/react/mysql'
import NodedotjsLogo from '@thesvg/react/nodedotjs'
import PagerdutyLogo from '@thesvg/react/pagerduty'
import PostgresqlLogo from '@thesvg/react/postgresql'
import RedisLogo from '@thesvg/react/redis'
import SlackLogo from '@thesvg/react/slack'
import TelegramLogo from '@thesvg/react/telegram'

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
  | 'clickhouse'
  | 'node'
  | 'go'
  | 'java'
  | 'discord'
  | 'slack'
  | 'telegram'
  | 'pagerduty'
  | 'microsoft-teams'

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
  clickhouse: ClickhouseLogo,
  node: NodedotjsLogo,
  go: GoLogo,
  java: JavaLogo,
  discord: DiscordLogo,
  slack: SlackLogo,
  telegram: TelegramLogo,
  pagerduty: PagerdutyLogo,
  'microsoft-teams': MicrosoftTeamsLogo,
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
  'clickhouse',
  'node',
  'go',
  'java',
  'discord',
  'slack',
  'telegram',
  'pagerduty',
  'microsoft-teams',
]

export interface BrandIconProps extends SVGProps<SVGSVGElement> {
  /** Which brand mark to render. */
  name: BrandIconName
}

// Renders a single brand/service logo. Size via a Tailwind `size-*`
// class on `className`; `aria-hidden="true"` by default since the mark
// is normally paired with a visible text label.
export function BrandIcon({
  name,
  'aria-hidden': ariaHidden,
  ...props
}: BrandIconProps) {
  // mysql/go's default variants don't work on a light background
  // (white-on-transparent; a wordmark lockup), so both use an explicit
  // non-default variant. Every other icon's default variant is fine.
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
  const Logo = BRAND_ICONS[name]
  return <Logo aria-hidden={ariaHidden ?? 'true'} {...props} />
}
