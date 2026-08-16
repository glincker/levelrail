import type { ComponentType, SVGProps } from 'react'
import DockerLogo from '@thesvg/react/docker'
import GoLogo from '@thesvg/react/go'
import JavaLogo from '@thesvg/react/java'
import MysqlLogo from '@thesvg/react/mysql'
import NodedotjsLogo from '@thesvg/react/nodedotjs'
import PostgresqlLogo from '@thesvg/react/postgresql'
import RedisLogo from '@thesvg/react/redis'

// Brand/service marks (Docker whale, Postgres elephant, Redis cube, MySQL
// dolphin, Node.js hexagon, Go gopher, Java cup), sourced from @thesvg/react
// per the creation-wizard-and-sidebar design spec's 2026-08-14 update:
// thesvg.org is the standing source for this category of icon, going
// forward, not just this one wizard's original 4 logos. node/go/java were
// added for the git-source build-pack picker (GitSourceCard.tsx), which
// needs to show which Railpack providers are actually supported
// (internal/build/railpack.go's supportedRailpackProviders) rather than a
// blank build-type dropdown. This is a separate concern from the app's UI
// icon system (@phosphor-icons/react/dist/ssr, locked by ADR 014 and this
// project's "No new icon sets" rule for nav/buttons/dialog controls): brand
// marks identify a third-party technology, UI chrome icons do not, and the
// two are never interchangeable.
//
// Each icon is imported from its own subpath (e.g. '@thesvg/react/docker')
// rather than the package's barrel export. Both are tree-shakeable in an
// ESM/Rollup build, but the subpath form is what the package's own README
// recommends for "best bundle size" and is what actually guarantees only
// the icons in BRAND_ICONS end up in the bundle, independent of whether a
// given bundler's dead-code elimination is perfect.
export type BrandIconName =
  | 'docker'
  | 'postgres'
  | 'redis'
  | 'mysql'
  | 'node'
  | 'go'
  | 'java'

const BRAND_ICONS: Record<
  BrandIconName,
  ComponentType<SVGProps<SVGSVGElement>>
> = {
  docker: DockerLogo,
  postgres: PostgresqlLogo,
  redis: RedisLogo,
  mysql: MysqlLogo,
  node: NodedotjsLogo,
  go: GoLogo,
  java: JavaLogo,
}

// All names in BRAND_ICONS, for callers that need to enumerate the
// available brand marks (e.g. rendering a picker grid) without hardcoding
// the list a second time. Kept in this file rather than split out (the
// same shape ThemeProvider.tsx's useTheme export and brandContext.ts
// explain): react-refresh/only-export-components flags this array export
// sharing a file with the BrandIcon component as a fast-refresh hazard,
// but there is no hazard in practice, it is a static, module-level
// constant with no state of its own.
// eslint-disable-next-line react-refresh/only-export-components
export const BRAND_ICON_NAMES: readonly BrandIconName[] = [
  'docker',
  'postgres',
  'redis',
  'mysql',
  'node',
  'go',
  'java',
]

export interface BrandIconProps extends SVGProps<SVGSVGElement> {
  /** Which brand mark to render. */
  name: BrandIconName
}

// Renders a single brand/service logo. Follows the same consumption
// convention as this codebase's @phosphor-icons usage (e.g. AppRow.tsx,
// NodeRow.tsx): size via a Tailwind `size-*` class on `className`,
// `aria-hidden="true"` by default since the mark is normally paired with
// a visible text label (app name, picker option label) and is decorative
// on its own. Callers that need the icon to carry the accessible name
// themselves (no adjacent text) can override aria-hidden and pass
// aria-label explicitly.
export function BrandIcon({
  name,
  'aria-hidden': ariaHidden,
  ...props
}: BrandIconProps) {
  // Every @thesvg/react icon's own "default" variant is meant to look
  // right wherever this codebase places it, and that holds for docker/
  // postgres/redis/node/java (their default variant paths carry real
  // brand colors: Docker's #008fe2, Redis's #912626, Node.js's green
  // gradient, Java's #5382A1, confirmed against the installed package's
  // own generated source, not guessed). MySQL and Go are the two
  // exceptions. MySQL's default variant paths are hardcoded white
  // (#FFF), built for a dark/brand-navy background, invisible on this
  // app's light picker cards. Its own "light" variant (a dark teal,
  // #00546B) is what @thesvg/react itself names for exactly this
  // light-background case. Go's default/light/dark variants are a
  // wordmark lockup (viewBox 207x78, white or black gopher with no
  // background rect of its own) meant to sit on a colored badge, not a
  // standalone square mark, so it renders through its "mono" variant
  // instead (currentColor, viewBox 24x24), the same plain glyph shape
  // as every other icon here. Both render through their own typed
  // component with the variant explicit, rather than through the
  // generic BRAND_ICONS map, which would need a widened `variant?:
  // string` prop type incompatible with each icon's own narrower,
  // per-icon variant union.
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
      <GoLogo
        variant="mono"
        aria-hidden={ariaHidden ?? 'true'}
        {...props}
      />
    )
  }
  const Logo = BRAND_ICONS[name]
  return <Logo aria-hidden={ariaHidden ?? 'true'} {...props} />
}
