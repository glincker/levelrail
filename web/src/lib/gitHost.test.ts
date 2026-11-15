import { describe, expect, it } from 'vitest'
import {
  GIT_CONNECT_CONFIG,
  gitAppPageUrl,
  gitCredentialIssue,
  gitHostIconName,
  gitRedirectHint,
  gitRedirectUri,
} from './gitHost'

describe('gitHostIconName', () => {
  it.each([
    ['https://github.com/org/repo.git', 'github'],
    ['https://www.github.com/org/repo', 'github'],
    ['github.com/org/repo', 'github'],
    ['git@github.com:org/repo.git', 'github'],
    ['ssh://git@gitlab.com/org/repo.git', 'gitlab'],
    ['https://bitbucket.org/org/repo', 'bitbucket'],
    ['https://github.com.evil.test/org/repo', null],
    ['https://evil.test/github.com/org/repo', null],
    ['https://evil.test/?u=gitlab.com', null],
    ['git@evil.test:github.com/repo.git', null],
    ['https://git.example.com/org/repo', null],
    ['', null],
  ])('%s -> %s', (url, want) => {
    expect(gitHostIconName(url)).toBe(want)
  })
})

describe('git connect config', () => {
  it('builds redirect URIs without a doubled slash', () => {
    expect(gitRedirectUri('https://lr.example.com/', 'gitlab')).toBe(
      'https://lr.example.com/api/v1/gitlab-app/callback',
    )
    expect(gitRedirectUri('http://localhost:5173', 'forgejo')).toBe(
      'http://localhost:5173/api/v1/forgejo-app/callback',
    )
  })

  it.each([
    [
      'gitlab',
      'https://git.corp.test/',
      'https://git.corp.test/-/user_settings/applications',
    ],
    ['gitlab', '', 'https://gitlab.com/-/user_settings/applications'],
    [
      'gitea',
      'https://gitea.test',
      'https://gitea.test/user/settings/applications',
    ],
    ['forgejo', '', 'https://codeberg.org/user/settings/applications'],
    ['bitbucket', '', 'https://bitbucket.org/account/workspaces/'],
    ['gitea', '', null],
    ['gitea', 'javascript:alert(1)', null],
    ['bitbucket_server', 'https://bb.test', null],
    ['azure_devops', 'https://dev.azure.com/', null],
  ] as const)('app page for %s at %s', (provider, base, want) => {
    expect(gitAppPageUrl(provider, base)).toBe(want)
  })

  it('marks token-only providers as non OAuth', () => {
    expect(GIT_CONNECT_CONFIG.bitbucket_server.oauth).toBe(false)
    expect(GIT_CONNECT_CONFIG.azure_devops.oauth).toBe(false)
    expect(GIT_CONNECT_CONFIG.gitlab.scopes).toEqual(['api'])
  })

  it.each([
    ['https://lr.example.com', 'https'],
    ['http://localhost:8080', 'loopback'],
    ['http://127.0.0.1:3000', 'loopback'],
    ['http://203.0.113.5', 'insecure'],
    ['http://lr.local', 'insecure'],
    ['not a url', 'insecure'],
  ] as const)('redirect hint for %s', (origin, want) => {
    expect(gitRedirectHint(origin)).toBe(want)
  })

  it('checks credentials by emptiness, spaces and shape only', () => {
    const hex = 'a'.repeat(64)
    expect(gitCredentialIssue('gitlab', 'id', '  ')).toBe('empty')
    expect(gitCredentialIssue('gitlab', 'id', `${hex} x`)).toBe('whitespace')
    expect(gitCredentialIssue('gitlab', 'id', 'short')).toBe('unusual')
    expect(gitCredentialIssue('gitlab', 'id', hex)).toBeNull()
    expect(gitCredentialIssue('gitlab', 'secret', `gloas-${hex}`)).toBeNull()
    expect(
      gitCredentialIssue('gitea', 'id', '6f1c2d3e-0a1b-4c5d-8e9f-0123456789ab'),
    ).toBeNull()
    expect(
      gitCredentialIssue('bitbucket', 'id', 'AbCdEfGhIjKlMnOpQr'),
    ).toBeNull()
    expect(gitCredentialIssue('bitbucket_server', 'id', 'anything')).toBeNull()
  })
})
