import type { Domain } from '../queries/domains'
import type {
  AttachInput,
  MoveInput,
  ParkInput,
  TargetKind,
} from '../queries/parkedDomains'

// Standalone and automatic hostnames are not in an app's domain sets, so
// they get neither park nor move. Environment sets are fully supported.
export function canParkOrMove(d: Pick<Domain, 'kind' | 'automatic'>): boolean {
  return d.kind !== 'standalone' && !d.automatic
}

// Target apps for a move.
// The owner stays a target when it has environments to move between.
export function moveTargets(
  apps: { name: string }[],
  from: string,
  allowSame = false,
): string[] {
  return apps.map((a) => a.name).filter((n) => allowSame || n !== from)
}

// Same-app moves are valid only between two different environment sets.
export function isNoopMove(m: {
  from: string
  to: string
  fromEnvironment?: string
  toEnvironment?: string
}): boolean {
  return (
    m.from === m.to && (m.fromEnvironment ?? '') === (m.toEnvironment ?? '')
  )
}

// The API defaults to an app, so only a static site names its kind.
function kindField<K extends string>(key: K, kind: TargetKind | undefined) {
  return kind === 'static_site' ? { [key]: kind } : {}
}

// Empty environment means the default list, so the field is left out.
export function parkRequestBody(input: ParkInput) {
  return {
    app: input.app,
    domain: input.domain,
    ...(input.note ? { note: input.note } : {}),
    ...(input.environment && input.targetKind !== 'static_site'
      ? { environment: input.environment }
      : {}),
    ...kindField('target_kind', input.targetKind),
  }
}

export function moveRequestBody(input: MoveInput) {
  return {
    domain: input.domain,
    from: input.from,
    to: input.to,
    confirm: input.confirm === true,
    ...(input.fromEnvironment && input.fromKind !== 'static_site'
      ? { from_environment: input.fromEnvironment }
      : {}),
    ...(input.toEnvironment && input.toKind !== 'static_site'
      ? { to_environment: input.toEnvironment }
      : {}),
    ...kindField('from_kind', input.fromKind),
    ...kindField('to_kind', input.toKind),
  }
}

export function attachRequestBody(input: AttachInput) {
  return {
    app: input.app,
    discard_settings: input.discardSettings === true,
    ...(input.environment && input.targetKind !== 'static_site'
      ? { environment: input.environment }
      : {}),
    ...kindField('target_kind', input.targetKind),
  }
}

export function isPrimaryDomain(
  domain: string,
  primary: string | undefined,
): boolean {
  return !!primary && primary.toLowerCase() === domain.toLowerCase()
}

// Fallback label for a restored-settings kind the locale has no entry for.
export function humanizeSettingKind(kind: string): string {
  return kind.replace(/[_-]+/g, ' ').trim()
}

export interface TargetOption {
  name: string
  kind: TargetKind
}

// Select values carry the kind so an app and a static site never collide.
export function encodeTarget(t: TargetOption): string {
  return `${t.kind}:${t.name}`
}

export function decodeTarget(value: string): TargetOption | null {
  const i = value.indexOf(':')
  if (i < 0) return null
  const kind = value.slice(0, i)
  const name = value.slice(i + 1)
  if ((kind !== 'app' && kind !== 'static_site') || name === '') return null
  return { kind, name }
}

// Apps first, then static sites; the source is left out unless allowed.
export function groupedTargets(
  apps: { name: string }[],
  sites: { name: string }[],
  source?: TargetOption,
  allowSame = false,
): { apps: TargetOption[]; sites: TargetOption[] } {
  const keep = (t: TargetOption) =>
    allowSame || !source || source.kind !== t.kind || source.name !== t.name
  const appNames = new Set(apps.map((a) => a.name))
  return {
    apps: apps
      .map((a): TargetOption => ({ name: a.name, kind: 'app' }))
      .filter(keep),
    sites: sites
      .filter((s) => !appNames.has(s.name))
      .map((s): TargetOption => ({ name: s.name, kind: 'static_site' }))
      .filter(keep),
  }
}

// Static sites keep no per-domain settings, so these moves lose them.
export function moveDropsSettings(
  fromKind: TargetKind | undefined,
  toKind: TargetKind | undefined,
): boolean {
  return fromKind !== 'static_site' && toKind === 'static_site'
}

export function attachDropsSettings(
  hasSettings: boolean,
  toKind: TargetKind | undefined,
): boolean {
  return hasSettings && toKind === 'static_site'
}
