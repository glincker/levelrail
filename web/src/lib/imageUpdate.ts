import { unpinnedImage } from './imageDigest'

export interface DockerHubRef {
  namespace: string
  repo: string
  tag: string
}

/** parseDockerHubImage splits an image reference that lives on Docker Hub; it returns null for any other registry. */
export function parseDockerHubImage(image: string): DockerHubRef | null {
  const bare = unpinnedImage(image)
  const slash = bare.indexOf('/')
  if (slash !== -1) {
    const first = bare.slice(0, slash)
    if (first.includes('.') || first.includes(':') || first === 'localhost') {
      return null
    }
  }
  const colon = bare.lastIndexOf(':')
  const hasTag = colon > bare.lastIndexOf('/')
  const name = hasTag ? bare.slice(0, colon) : bare
  const tag = hasTag ? bare.slice(colon + 1) : 'latest'
  const parts = name.split('/')
  if (parts.length === 1) return { namespace: 'library', repo: name, tag }
  if (parts.length === 2) {
    return { namespace: parts[0] ?? '', repo: parts[1] ?? '', tag }
  }
  return null
}

interface ParsedTag {
  prefix: string
  nums: number[]
  suffix: string
}

const TAG_PATTERN = /^(v?)(\d+(?:\.\d+){0,3})(.*)$/
const PRERELEASE = /(rc|alpha|beta|dev|nightly|snapshot|preview|canary)/i

function parseTag(tag: string): ParsedTag | null {
  const m = TAG_PATTERN.exec(tag)
  if (!m) return null
  return {
    prefix: m[1] ?? '',
    nums: (m[2] ?? '').split('.').map(Number),
    suffix: m[3] ?? '',
  }
}

function compareNums(a: number[], b: number[]): number {
  const len = Math.max(a.length, b.length)
  for (let i = 0; i < len; i++) {
    const diff = (a[i] ?? 0) - (b[i] ?? 0)
    if (diff !== 0) return diff
  }
  return 0
}

/** latestNewerTag returns the highest tag in the same family as current that is strictly newer, or null. Same family means the same v prefix and suffix (for example -alpine), and no pre-release unless current is one. */
export function latestNewerTag(
  current: string,
  tags: readonly string[],
): string | null {
  const cur = parseTag(current)
  if (!cur) return null
  const currentIsPrerelease = PRERELEASE.test(cur.suffix)
  let best: { tag: string; parsed: ParsedTag } | null = null
  for (const tag of tags) {
    const parsed = parseTag(tag)
    if (!parsed) continue
    if (parsed.prefix !== cur.prefix || parsed.suffix !== cur.suffix) continue
    if (!currentIsPrerelease && PRERELEASE.test(parsed.suffix)) continue
    if (parsed.nums.length < cur.nums.length) continue
    if (compareNums(parsed.nums, cur.nums) <= 0) continue
    if (!best || compareNums(parsed.nums, best.parsed.nums) > 0) {
      best = { tag, parsed }
    }
  }
  return best?.tag ?? null
}
