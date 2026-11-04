export const ORG_SLUG_MIN = 3
export const ORG_SLUG_MAX = 40

export type SlugProblem = 'length' | 'chars' | null

// 3 to 40 chars of lowercase letters, digits and single hyphens (not at the ends).
export function slugProblem(slug: string): SlugProblem {
  if (slug.length < ORG_SLUG_MIN || slug.length > ORG_SLUG_MAX) return 'length'
  if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(slug)) return 'chars'
  return null
}

export function orgInitials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  const first = words[0]?.[0] ?? '?'
  const second = words.length > 1 ? (words[1]?.[0] ?? '') : ''
  return (first + second).toUpperCase()
}

// Static class names so Tailwind can see them; the id hash picks a stable one.
const AVATAR_TONES = [
  'bg-tone-info-soft text-tone-info-fg',
  'bg-tone-success-soft text-tone-success-fg',
  'bg-tone-warning-soft text-tone-warning-fg',
  'bg-tone-accent-soft text-tone-accent-fg',
  'bg-tone-danger-soft text-tone-danger-fg',
  'bg-tone-neutral-soft text-tone-neutral-fg',
] as const

export function avatarToneClass(id: string): string {
  let h = 0
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return AVATAR_TONES[h % AVATAR_TONES.length] ?? AVATAR_TONES[0]
}
