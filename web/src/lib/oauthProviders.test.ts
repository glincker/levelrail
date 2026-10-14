import { describe, expect, it } from 'vitest'
import { brandKeyFor, callbackUrl, connectionState } from './oauthProviders'

describe('connectionState', () => {
  it.each([
    [{ enabled: true, client_id: 'id', has_client_secret: true }, 'connected'],
    [{ enabled: false, client_id: 'id', has_client_secret: true }, 'disabled'],
    [
      { enabled: true, client_id: 'id', has_client_secret: false },
      'unconfigured',
    ],
    [{ enabled: true, client_id: '', has_client_secret: true }, 'unconfigured'],
    [{ enabled: false, has_client_secret: false }, 'unconfigured'],
  ] as const)('case %#', (input, want) => {
    expect(connectionState(input)).toBe(want)
  })
})

describe('callbackUrl', () => {
  it.each([
    [
      'https://ops.example.com',
      'google',
      'https://ops.example.com/api/v1/auth/oauth/google/callback',
    ],
    [
      'https://ops.example.com/',
      'github',
      'https://ops.example.com/api/v1/auth/oauth/github/callback',
    ],
    [
      'http://localhost:9100',
      'oidc',
      'http://localhost:9100/api/v1/auth/oauth/oidc/callback',
    ],
  ] as const)('%s %s', (origin, provider, want) => {
    expect(callbackUrl(origin, provider)).toBe(want)
  })
})

describe('brandKeyFor', () => {
  it.each([
    ['google', 'google'],
    ['oidc', 'openid'],
    ['gitlab', 'gitlab'],
    ['bitbucket', 'bitbucket'],
    ['discord', 'discord'],
    ['slack', 'slack'],
    ['linkedin', 'linkedin'],
    ['twitch', 'twitch'],
    ['facebook', 'facebook'],
    ['custom', 'custom'],
  ] as const)('%s', (p, want) => {
    expect(brandKeyFor(p)).toBe(want)
  })
})
