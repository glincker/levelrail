export type TokenHost = 'gitlab' | 'bitbucket' | 'gitea' | 'forgejo'

export const TOKEN_HOSTS: readonly TokenHost[] = [
  'gitlab',
  'bitbucket',
  'gitea',
  'forgejo',
]

export function isTokenHost(provider: string): provider is TokenHost {
  return (TOKEN_HOSTS as readonly string[]).includes(provider)
}

export interface TokenGuide {
  /** Bitbucket Cloud has one fixed address; the others ask for a server URL. */
  selfHosted: boolean
  defaultUrl: string
  /** Scope names exactly as the host's token form shows them. */
  scopes: readonly string[]
  /** The page on the host where the token is created. */
  tokenPage: (serverUrl: string, tokenName: string) => string
  /** Bitbucket Cloud API tokens are pasted as email:token. */
  tokenFormat: 'token' | 'emailToken'
}

const trimUrl = (url: string) => url.trim().replace(/\/+$/, '')

export const TOKEN_GUIDES: Record<TokenHost, TokenGuide> = {
  gitlab: {
    selfHosted: true,
    defaultUrl: 'https://gitlab.com',
    scopes: ['api'],
    tokenFormat: 'token',
    tokenPage: (url, name) =>
      `${trimUrl(url)}/-/user_settings/personal_access_tokens?name=${encodeURIComponent(name)}&scopes=api`,
  },
  bitbucket: {
    selfHosted: false,
    defaultUrl: 'https://bitbucket.org',
    scopes: [
      'read:user:bitbucket',
      'read:repository:bitbucket',
      'write:repository:bitbucket',
      'read:pullrequest:bitbucket',
      'write:pullrequest:bitbucket',
      'read:webhook:bitbucket',
      'write:webhook:bitbucket',
    ],
    tokenFormat: 'emailToken',
    tokenPage: () =>
      'https://id.atlassian.com/manage-profile/security/api-tokens',
  },
  gitea: {
    selfHosted: true,
    defaultUrl: '',
    scopes: ['read:user', 'write:repository'],
    tokenFormat: 'token',
    tokenPage: (url) => `${trimUrl(url)}/user/settings/applications`,
  },
  forgejo: {
    selfHosted: true,
    defaultUrl: 'https://codeberg.org',
    scopes: ['read:user', 'write:repository'],
    tokenFormat: 'token',
    tokenPage: (url) => `${trimUrl(url)}/user/settings/applications`,
  },
}
