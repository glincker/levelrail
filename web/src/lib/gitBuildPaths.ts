// Client-side mirror of internal/spec's NormalizeRepoPath and
// ResolveGitBuild. The server is authoritative; this only gives the form
// instant, plain feedback before a save.

import type { GitSourceBuildType } from '../types/gitSource'

export type BuildPathProblem = 'absolute' | 'parent' | 'outside'

export function normalizeRepoPath(input: string): string {
  return input
    .trim()
    .split('/')
    .filter((seg) => seg !== '' && seg !== '.')
    .join('/')
}

function escapes(path: string): boolean {
  let depth = 0
  for (const seg of path.split('/')) {
    if (seg === '..') depth -= 1
    else if (seg !== '') depth += 1
    if (depth < 0) return true
  }
  return false
}

function isAbsolute(input: string): boolean {
  const p = input.trim()
  return p.startsWith('/') || /^[A-Za-z]:/.test(p)
}

export function validateBuildPaths(
  buildType: GitSourceBuildType,
  baseDirectory: string,
  buildPath: string,
): { field: 'base' | 'path'; problem: BuildPathProblem } | null {
  if (isAbsolute(baseDirectory)) return { field: 'base', problem: 'absolute' }
  if (escapes(normalizeRepoPath(baseDirectory))) {
    return { field: 'base', problem: 'parent' }
  }
  if (isAbsolute(buildPath)) return { field: 'path', problem: 'absolute' }
  if (escapes(normalizeRepoPath(buildPath))) {
    return { field: 'path', problem: 'parent' }
  }
  const base = normalizeRepoPath(baseDirectory)
  const p = normalizeRepoPath(buildPath)
  if (buildType !== 'railpack' && base !== '' && p !== '') {
    if (!p.startsWith(base + '/')) return { field: 'path', problem: 'outside' }
  }
  return null
}

export interface ResolvedBuildView {
  context: string | null
  dockerfile: string | null
}

// resolveBuildView returns the context directory (null means the
// repository root) and, for a Dockerfile build, the Dockerfile path
// relative to the repository root.
export function resolveBuildView(
  buildType: GitSourceBuildType,
  baseDirectory: string,
  buildPath: string,
): ResolvedBuildView {
  const base = normalizeRepoPath(baseDirectory)
  const p = normalizeRepoPath(buildPath)
  const context = base === '' ? null : base
  if (buildType !== 'dockerfile') return { context, dockerfile: null }
  if (p !== '') return { context, dockerfile: p }
  return {
    context,
    dockerfile: base === '' ? 'Dockerfile' : `${base}/Dockerfile`,
  }
}
