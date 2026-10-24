import { describe, expect, it } from 'vitest'
import type { NormalizedRepoOption } from './gitRepoOptions'
import { filterRepos } from './repoFilter'

function repo(
  path: string,
  extra: Partial<NormalizedRepoOption> = {},
): NormalizedRepoOption {
  return {
    key: path,
    displayPath: path,
    private: false,
    defaultBranch: 'main',
    cloneUrl: `https://github.com/${path}.git`,
    providerRef: { kind: 'github' as const, owner: 'a', repo: 'b' },
    ...extra,
  }
}

const repos = [
  repo('acme/shop', { pushedAt: '2026-01-01', language: 'TypeScript' }),
  repo('acme/docs', { pushedAt: '2026-02-01', language: 'MDX' }),
]

describe('filterRepos', () => {
  it('sorts by last push and filters by name or language', () => {
    expect(filterRepos(repos, '', null).map((r) => r.displayPath)).toEqual([
      'acme/docs',
      'acme/shop',
    ])
    expect(filterRepos(repos, 'mdx', null)).toHaveLength(1)
  })

  it('narrows a pasted URL to its own connected repository', () => {
    const hit = filterRepos(repos, 'ignored', 'https://github.com/acme/shop')
    expect(hit.map((r) => r.displayPath)).toEqual(['acme/shop'])
    expect(filterRepos(repos, '', 'https://github.com/other/x')).toEqual([])
  })
})
