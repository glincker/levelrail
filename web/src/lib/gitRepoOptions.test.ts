import { describe, expect, it } from 'vitest'
import type { GitRepoItem } from '../types/gitRepos'
import {
  fromGitRepoItem,
  normalizeRepoUrl,
  providerOf,
  toRepoOptions,
} from './gitRepoOptions'
import { filterRepos } from './repoFilter'

function row(extra: Partial<GitRepoItem> = {}): GitRepoItem {
  return {
    key: 'PRJ/api',
    full_name: 'PRJ/api',
    name: 'api',
    owner: 'PRJ',
    private: true,
    archived: false,
    fork: false,
    default_branch: 'main',
    clone_url: 'https://git.example.com/scm/prj/api.git',
    connection_id: 'conn-1',
    ...extra,
  }
}

describe('fromGitRepoItem', () => {
  it('adapts Forgejo rows through their connection', () => {
    const o = fromGitRepoItem('forgejo', row({ full_name: 'me/api' }))
    expect(o.providerRef).toEqual({
      kind: 'forgejo',
      connectionId: 'conn-1',
      repo: 'me/api',
    })
  })

  it('adapts Bitbucket Server rows with PROJECT/slug as the repo', () => {
    const o = fromGitRepoItem('bitbucket_server', row())
    expect(o.providerRef).toEqual({
      kind: 'bitbucket_server',
      connectionId: 'conn-1',
      repo: 'PRJ/api',
    })
  })

  it('never returns undefined for a provider this build does not know', () => {
    const o = fromGitRepoItem('sourcehut' as never, row())
    expect(o.providerRef).toMatchObject({
      connectionId: 'conn-1',
      repo: 'PRJ/api',
    })
  })
})

describe('providerOf', () => {
  it('treats empty, missing and non-string providers as github', () => {
    expect(providerOf(row({ provider: '' as never }))).toBe('github')
    expect(providerOf(row())).toBe('github')
    expect(providerOf(row({ provider: 7 as never }))).toBe('github')
    expect(providerOf(row({ provider: 'forgejo' }))).toBe('forgejo')
  })
})

describe('toRepoOptions', () => {
  it('skips null, undefined and clone-less rows', () => {
    const out = toRepoOptions([
      null,
      undefined,
      row({ clone_url: undefined as never }),
      row({ provider: 'forgejo' }),
    ])
    expect(out).toHaveLength(1)
  })
})

describe('filterRepos with Forgejo and Bitbucket Server lists', () => {
  const repos = toRepoOptions([
    row({
      provider: 'forgejo',
      key: 'a',
      clone_url: 'https://f.example.com/me/a.git',
    }),
    row({ provider: 'bitbucket_server', key: 'b' }),
    row({
      provider: 'nope' as never,
      key: 'c',
      clone_url: 'https://x.example.com/o/c',
    }),
  ])

  it('does not throw on a pasted URL and matches the right row', () => {
    const hit = filterRepos(repos, '', 'https://f.example.com/me/a')
    expect(hit.map((r) => r.cloneUrl)).toEqual([
      'https://f.example.com/me/a.git',
    ])
  })

  it('tolerates undefined entries in the list', () => {
    const dirty = [undefined, ...repos] as never
    expect(() => filterRepos(dirty, 'prj', null)).not.toThrow()
    expect(() => filterRepos(dirty, '', 'https://a/b/c')).not.toThrow()
  })
})

describe('normalizeRepoUrl', () => {
  it('matches ssh and https clone URLs of one repository', () => {
    const https = normalizeRepoUrl('https://git.example.com/owner/repo')
    expect(normalizeRepoUrl('git@git.example.com:owner/repo.git')).toBe(https)
    expect(normalizeRepoUrl('ssh://git@git.example.com/owner/repo.git')).toBe(
      https,
    )
  })

  it('keeps a host port and tolerates bad input', () => {
    expect(normalizeRepoUrl('https://h:8443/o/r.git')).toBe('h:8443/o/r')
    expect(normalizeRepoUrl('')).toBe('')
    expect(normalizeRepoUrl(undefined)).toBe('')
    expect(normalizeRepoUrl(5)).toBe('')
  })
})
