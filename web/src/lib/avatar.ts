// Literal class strings, so Tailwind's scanner keeps every tone.
export const AVATAR_TONES = [
  'bg-rose-100 text-rose-900 dark:bg-rose-900/40 dark:text-rose-200',
  'bg-orange-100 text-orange-900 dark:bg-orange-900/40 dark:text-orange-200',
  'bg-amber-100 text-amber-900 dark:bg-amber-900/40 dark:text-amber-200',
  'bg-emerald-100 text-emerald-900 dark:bg-emerald-900/40 dark:text-emerald-200',
  'bg-teal-100 text-teal-900 dark:bg-teal-900/40 dark:text-teal-200',
  'bg-sky-100 text-sky-900 dark:bg-sky-900/40 dark:text-sky-200',
  'bg-violet-100 text-violet-900 dark:bg-violet-900/40 dark:text-violet-200',
  'bg-fuchsia-100 text-fuchsia-900 dark:bg-fuchsia-900/40 dark:text-fuchsia-200',
] as const

function hashSeed(seed: string): number {
  let h = 5381
  for (let i = 0; i < seed.length; i += 1) {
    h = ((h << 5) + h + seed.charCodeAt(i)) >>> 0
  }
  return h
}

// The same seed always gives the same tone, so an avatar never changes color.
export function avatarTone(seed: string): string {
  return AVATAR_TONES[hashSeed(seed) % AVATAR_TONES.length] ?? AVATAR_TONES[0]
}

export function avatarInitials(name: string): string {
  const words = name
    .replace(/@.*$/, '')
    .split(/[\s._-]+/)
    .filter((w) => w !== '')
  const firstWord = words[0]
  if (firstWord === undefined) return '?'
  const lastWord = words.length > 1 ? words[words.length - 1] : undefined
  const first = Array.from(firstWord)[0] ?? '?'
  const last = lastWord === undefined ? '' : (Array.from(lastWord)[0] ?? '')
  return (first + last).toUpperCase()
}

// Gravatar accepts a SHA-256 of the trimmed, lower-cased email; d=404 makes a
// missing picture an error the avatar can fall back from.
export async function gravatarUrl(email: string, size = 96): Promise<string> {
  const data = new TextEncoder().encode(email.trim().toLowerCase())
  const digest = await crypto.subtle.digest('SHA-256', data)
  const hex = Array.from(new Uint8Array(digest), (b) =>
    b.toString(16).padStart(2, '0'),
  ).join('')
  return `https://www.gravatar.com/avatar/${hex}?s=${String(size)}&d=404`
}
