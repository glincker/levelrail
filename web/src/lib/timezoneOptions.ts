// Zone labels for the timezone picker: current local time and UTC offset.

export const COMMON_ZONES: readonly string[] = [
  'UTC',
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Africa/Johannesburg',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Australia/Sydney',
  'Pacific/Auckland',
]

export function zoneClock(zone: string, now: Date): string {
  try {
    return new Intl.DateTimeFormat(undefined, {
      timeZone: zone,
      hour: '2-digit',
      minute: '2-digit',
    }).format(now)
  } catch {
    return ''
  }
}

// "UTC+5:30" style offset; empty when the runtime cannot name it.
export function zoneOffset(zone: string, now: Date): string {
  try {
    const part = new Intl.DateTimeFormat('en', {
      timeZone: zone,
      timeZoneName: 'shortOffset',
    })
      .formatToParts(now)
      .find((p) => p.type === 'timeZoneName')
    return (part?.value ?? '').replace('GMT', 'UTC')
  } catch {
    return ''
  }
}

export function zoneDisplayName(zone: string): string {
  return zone.replace(/_/g, ' ')
}

export interface ZoneOption {
  value: string
  label: string
  trailing: string
  keywords: string
  group: 'common' | 'all'
}

// Common zones first, then every other zone; the browser zone is added by the caller.
export function buildZoneOptions(
  all: readonly string[],
  now: Date,
): ZoneOption[] {
  const known = new Set(all)
  const common = COMMON_ZONES.filter((z) => z === 'UTC' || known.has(z))
  const commonSet = new Set(common)
  const make = (z: string, group: ZoneOption['group']): ZoneOption => {
    const offset = zoneOffset(z, now)
    return {
      value: z,
      label: zoneDisplayName(z),
      trailing: [zoneClock(z, now), offset].filter(Boolean).join(' '),
      keywords: `${z} ${offset}`,
      group,
    }
  }
  return [
    ...common.map((z) => make(z, 'common')),
    ...all.filter((z) => !commonSet.has(z)).map((z) => make(z, 'all')),
  ]
}
