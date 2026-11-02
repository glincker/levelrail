import type { Domain } from '../queries/domains'

// Park and move act on an app's default domain list only. Standalone and
// automatic hostnames are not in that list, so they get neither action.
export function canParkOrMove(d: Pick<Domain, 'kind' | 'automatic'>): boolean {
  return d.kind !== 'standalone' && !d.automatic
}

// Target apps for a move: every app except the current owner.
export function moveTargets(apps: { name: string }[], from: string): string[] {
  return apps.map((a) => a.name).filter((n) => n !== from)
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
