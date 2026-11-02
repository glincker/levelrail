import { describe, expect, it } from 'vitest'
import {
  canParkOrMove,
  attachDropsSettings,
  decodeTarget,
  encodeTarget,
  groupedTargets,
  moveDropsSettings,
  attachRequestBody,
  humanizeSettingKind,
  isNoopMove,
  isPrimaryDomain,
  moveRequestBody,
  moveTargets,
  parkRequestBody,
} from './domainLifecycle'

describe('domainLifecycle', () => {
  it('allows park and move for app domains only', () => {
    expect(canParkOrMove({ kind: 'app' })).toBe(true)
    expect(canParkOrMove({})).toBe(true)
    expect(canParkOrMove({ kind: 'standalone' })).toBe(false)
    expect(canParkOrMove({ kind: 'app', automatic: true })).toBe(false)
  })
  it('lists move targets without the owner', () => {
    expect(moveTargets([{ name: 'a' }, { name: 'b' }], 'a')).toEqual(['b'])
  })
  it('humanizes restored setting kinds', () => {
    expect(humanizeSettingKind('error_pages')).toBe('error pages')
    expect(humanizeSettingKind('waf')).toBe('waf')
  })
  it('matches the primary domain case-insensitively', () => {
    expect(isPrimaryDomain('Glinr.com', 'glinr.com')).toBe(true)
    expect(isPrimaryDomain('a.com', undefined)).toBe(false)
  })
  it('keeps the owner as a target when it has environments', () => {
    expect(moveTargets([{ name: 'a' }, { name: 'b' }], 'a', true)).toEqual([
      'a',
      'b',
    ])
  })
  it('treats a same-app move between equal sets as a no-op', () => {
    expect(isNoopMove({ from: 'a', to: 'a' })).toBe(true)
    expect(isNoopMove({ from: 'a', to: 'a', toEnvironment: 'uat' })).toBe(false)
    expect(
      isNoopMove({
        from: 'a',
        to: 'a',
        fromEnvironment: 'uat',
        toEnvironment: 'uat',
      }),
    ).toBe(true)
  })
  it('builds park payloads with the optional environment', () => {
    expect(parkRequestBody({ app: 'a', domain: 'x.com' })).toEqual({
      app: 'a',
      domain: 'x.com',
    })
    expect(
      parkRequestBody({ app: 'a', domain: 'x.com', environment: 'env-1' }),
    ).toEqual({ app: 'a', domain: 'x.com', environment: 'env-1' })
  })
  it('builds move payloads with source and target environments', () => {
    expect(moveRequestBody({ domain: 'x.com', from: 'a', to: 'b' })).toEqual({
      domain: 'x.com',
      from: 'a',
      to: 'b',
      confirm: false,
    })
    expect(
      moveRequestBody({
        domain: 'x.com',
        from: 'a',
        to: 'a',
        confirm: true,
        fromEnvironment: 'dev',
        toEnvironment: 'uat',
      }),
    ).toEqual({
      domain: 'x.com',
      from: 'a',
      to: 'a',
      confirm: true,
      from_environment: 'dev',
      to_environment: 'uat',
    })
  })
  it('builds attach payloads with the optional environment', () => {
    expect(attachRequestBody({ domain: 'x.com', app: 'a' })).toEqual({
      app: 'a',
      discard_settings: false,
    })
    expect(
      attachRequestBody({
        domain: 'x.com',
        app: 'a',
        discardSettings: true,
        environment: 'env-1',
      }),
    ).toEqual({ app: 'a', discard_settings: true, environment: 'env-1' })
  })
  it('names the kind only for static sites', () => {
    expect(parkRequestBody({ app: 's', domain: 'a.com' })).toEqual({
      app: 's',
      domain: 'a.com',
    })
    expect(
      parkRequestBody({
        app: 's',
        domain: 'a.com',
        environment: 'uat',
        targetKind: 'static_site',
      }),
    ).toEqual({ app: 's', domain: 'a.com', target_kind: 'static_site' })
    expect(
      moveRequestBody({
        domain: 'a.com',
        from: 'x',
        to: 's',
        toEnvironment: 'uat',
        fromKind: 'app',
        toKind: 'static_site',
      }),
    ).toEqual({
      domain: 'a.com',
      from: 'x',
      to: 's',
      confirm: false,
      to_kind: 'static_site',
    })
    expect(
      attachRequestBody({
        domain: 'a.com',
        app: 's',
        targetKind: 'static_site',
      }),
    ).toEqual({ app: 's', discard_settings: false, target_kind: 'static_site' })
  })
  it('round-trips target values', () => {
    const t = { name: 'docs', kind: 'static_site' as const }
    expect(decodeTarget(encodeTarget(t))).toEqual(t)
    expect(decodeTarget('nope')).toBeNull()
  })
  it('groups apps and static sites without the source', () => {
    const g = groupedTargets(
      [{ name: 'a' }, { name: 'b' }],
      [{ name: 'a' }, { name: 'docs' }],
      { name: 'a', kind: 'app' },
    )
    expect(g.apps.map((t) => t.name)).toEqual(['b'])
    expect(g.sites.map((t) => t.name)).toEqual(['docs'])
  })
  it('flags moves and attaches that drop settings', () => {
    expect(moveDropsSettings('app', 'static_site')).toBe(true)
    expect(moveDropsSettings('static_site', 'static_site')).toBe(false)
    expect(moveDropsSettings('static_site', 'app')).toBe(false)
    expect(attachDropsSettings(true, 'static_site')).toBe(true)
    expect(attachDropsSettings(false, 'static_site')).toBe(false)
  })
})
