export type VersionParts =
  | { kind: 'release'; version: string }
  | { kind: 'dev'; base: string; commits: number; sha: string; dirty: boolean }
  | { kind: 'raw'; version: string }

const DESCRIBE =
  /^(v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+?)?)-(\d+)-g([0-9a-f]{7,40})(-dirty)?$/

export function parseVersion(raw: string): VersionParts {
  const v = raw.trim()
  const m = DESCRIBE.exec(v)
  if (m) {
    return {
      kind: 'dev',
      base: m[1] ?? '',
      commits: Number(m[2]),
      sha: m[3] ?? '',
      dirty: m[4] !== undefined,
    }
  }
  if (/^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(v)) {
    return { kind: 'release', version: v }
  }
  return { kind: 'raw', version: v }
}
