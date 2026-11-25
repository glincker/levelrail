export type PagesFailHint =
  | 'outOfMemory'
  | 'timeout'
  | 'installFailed'
  | 'nodeVersion'
  | 'missingBuildCommand'
  | 'outputDirMissing'
  | 'outputDirEmpty'

// Order matters: resource limits and the specific script errors come before
// the broad install shapes (npm prints "npm ERR!" for a missing script too).
const RULES: ReadonlyArray<readonly [PagesFailHint, RegExp]> = [
  [
    'outOfMemory',
    /heap out of memory|javascript heap|signal: killed|exit code 137|oomkilled|cannot allocate memory/i,
  ],
  [
    'timeout',
    /build timed out|timed out after|deadline exceeded|exceeded the build time/i,
  ],
  [
    'nodeVersion',
    /unsupported engine|engine "node" is incompatible|requires node|node version .*(not supported|required)|wanted: \{"node"/i,
  ],
  [
    'missingBuildCommand',
    /missing script: "?build|no build command|build command .*(not set|empty)|sh: \d+: .* not found/i,
  ],
  [
    'outputDirEmpty',
    /output directory .*(is empty|contains no files)|no files to publish|nothing to publish|build output is empty/i,
  ],
  [
    'outputDirMissing',
    /output directory .*(not found|does not exist|missing)|no such file or directory.*(dist|build|public|out)\b|publish directory .*(not found|does not exist)/i,
  ],
  [
    'installFailed',
    /npm err!|npm error|err_pnpm|yarn error|eresolve|e404|enotfound .*registry|lockfile .*(out of date|frozen)|install failed/i,
  ],
]

/** Picks one plain-language fix hint from the tail of a failed build log, or null when no known shape matches. */
export function pagesFailHint(lines: readonly string[]): PagesFailHint | null {
  for (const [hint, re] of RULES) {
    if (lines.some((l) => re.test(l))) return hint
  }
  return null
}
