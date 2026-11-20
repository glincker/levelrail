// The zone lookup error names the zones the provider lists, so the base
// domain field can offer them. Names only: no extra endpoint is involved.
const LISTED_RE = /the provider lists (.+?)(?: and \d+ more)?$/

// The structured zones field wins; the message parse stays for older servers.
export function listedZoneNames(
  message: string | undefined,
  zones?: string[],
): string[] {
  if (zones && zones.length > 0) {
    return zones
      .map((n) => n.trim().toLowerCase())
      .filter((n) => n.includes('.'))
  }
  const match = message ? LISTED_RE.exec(message.trim()) : null
  const list = match?.[1]
  if (!list) return []
  return list
    .split(',')
    .map((n) => n.trim().toLowerCase())
    .filter((n) => n.includes('.'))
}

// Actions the zone lookup and DNS-01 record writes need on the provider.
export const ROUTE53_IAM_ACTIONS = [
  'route53:ListHostedZones',
  'route53:ListResourceRecordSets',
  'route53:ChangeResourceRecordSets',
  'route53:GetChange',
]
