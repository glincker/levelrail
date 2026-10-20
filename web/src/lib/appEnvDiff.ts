export interface AppEnvDiffEntry {
  key: string
  a?: string
  b?: string
  secret: boolean
}

export interface AppEnvDiff {
  onlyA: AppEnvDiffEntry[]
  onlyB: AppEnvDiffEntry[]
  differ: AppEnvDiffEntry[]
  same: number
}

interface Side {
  env: Record<string, string>
  secretKeys: string[]
}

/** diffAppEnv compares two apps' env. Secret values are never read, so secrets match by key only. */
export function diffAppEnv(a: Side, b: Side): AppEnvDiff {
  const aSecret = new Set(a.secretKeys)
  const bSecret = new Set(b.secretKeys)
  const keys = [
    ...new Set([
      ...Object.keys(a.env),
      ...Object.keys(b.env),
      ...aSecret,
      ...bSecret,
    ]),
  ].sort()
  const out: AppEnvDiff = { onlyA: [], onlyB: [], differ: [], same: 0 }
  for (const key of keys) {
    const aHas = key in a.env || aSecret.has(key)
    const bHas = key in b.env || bSecret.has(key)
    const secret = aSecret.has(key) || bSecret.has(key)
    if (aHas && !bHas) {
      out.onlyA.push({ key, secret, a: secret ? undefined : a.env[key] })
    } else if (bHas && !aHas) {
      out.onlyB.push({ key, secret, b: secret ? undefined : b.env[key] })
    } else if (aSecret.has(key) !== bSecret.has(key)) {
      out.differ.push({ key, secret: true })
    } else if (secret || a.env[key] === b.env[key]) {
      out.same += 1
    } else {
      out.differ.push({ key, secret: false, a: a.env[key], b: b.env[key] })
    }
  }
  return out
}
