import type { Domain } from '../queries/domains'
import type {
  AttachInput,
  MoveInput,
  ParkInput,
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

// Empty environment means the default list, so the field is left out.
export function parkRequestBody(input: ParkInput) {
  return {
    app: input.app,
    domain: input.domain,
    ...(input.note ? { note: input.note } : {}),
    ...(input.environment ? { environment: input.environment } : {}),
  }
}

export function moveRequestBody(input: MoveInput) {
  return {
    domain: input.domain,
    from: input.from,
    to: input.to,
    confirm: input.confirm === true,
    ...(input.fromEnvironment
      ? { from_environment: input.fromEnvironment }
      : {}),
    ...(input.toEnvironment ? { to_environment: input.toEnvironment } : {}),
  }
}

export function attachRequestBody(input: AttachInput) {
  return {
    app: input.app,
    discard_settings: input.discardSettings === true,
    ...(input.environment ? { environment: input.environment } : {}),
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
