import { describe, expect, it } from 'vitest'
import { pagesFailHint } from './pagesFailHint'

describe('pagesFailHint', () => {
  const cases: ReadonlyArray<[string, string[], string | null]> = [
    [
      'oom',
      [
        'FATAL ERROR: Reached heap limit Allocation failed - JavaScript heap out of memory',
      ],
      'outOfMemory',
    ],
    ['oom kill', ['build step exited: signal: killed'], 'outOfMemory'],
    ['timeout', ['build timed out after 600s'], 'timeout'],
    [
      'node version',
      ['error vite@6.0.0: The engine "node" is incompatible with this module.'],
      'nodeVersion',
    ],
    [
      'npm install',
      ['npm ERR! code ERESOLVE', 'npm ERR! Could not resolve dependency'],
      'installFailed',
    ],
    [
      'pnpm lockfile',
      ['ERR_PNPM_OUTDATED_LOCKFILE  Cannot install with frozen-lockfile'],
      'installFailed',
    ],
    [
      'no build script',
      ['npm ERR! Missing script: "build"'],
      'missingBuildCommand',
    ],
    ['build cmd', ['sh: 1: astro: command not found'], 'missingBuildCommand'],
    [
      'dir missing',
      ['output directory "dist" not found after build'],
      'outputDirMissing',
    ],
    ['dir empty', ['output directory dist is empty'], 'outputDirEmpty'],
    ['unknown', ['Error: something odd happened'], null],
    ['no lines', [], null],
  ]
  it.each(cases)('%s', (_n, lines, want) => {
    expect(pagesFailHint(lines)).toBe(want)
  })
})
