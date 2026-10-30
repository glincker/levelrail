// Stable per-address avatar: initials plus one of a few token-based tones.
const TONES = [
  'bg-tone-accent/15 text-tone-accent',
  'bg-tone-info/15 text-tone-info',
  'bg-tone-success/15 text-tone-success',
  'bg-tone-warning/15 text-tone-warning',
  'bg-tone-danger/15 text-tone-danger',
  'bg-primary/10 text-primary',
] as const

export function avatarTone(address: string): string {
  let h = 0
  for (const ch of address.toLowerCase()) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return TONES[h % TONES.length] ?? TONES[0]
}

export function avatarInitials(localpart: string): string {
  const parts = localpart.split(/[._+-]+/).filter(Boolean)
  const [a = '?', b] = parts
  return (b ? a.slice(0, 1) + b.slice(0, 1) : a.slice(0, 2)).toUpperCase()
}
