import { describe, expect, it } from 'vitest'
import type { Domain } from '../../../queries/domains'
import {
  functionHost,
  nameFromImage,
  nameFromRepoUrl,
  normalizePath,
  previewUrls,
  routableDomains,
  suggestName,
  toKebab,
  validate,
  type FlowValues,
} from './logic'
import { findPreset, presetDefaults } from './presets'

function domain(over: Partial<Domain>): Domain {
  return {
    domain: 'example.com',
    service_name: 'web',
    waf_enabled: false,
    has_redirect: false,
    maintenance_enabled: false,
    has_basic_auth: false,
    ...over,
  }
}

const valid: FlowValues = {
  source: 'repo',
  repoUrl: 'https://github.com/you/hello',
  ref: 'main',
  directory: '',
  image: '',
  name: 'hello',
  port: '8080',
  idleChoice: '10',
  customIdle: '10',
  routeDomain: '',
  routePath: '',
}

describe('preset prefill', () => {
  it('prefills runtime, idle period and suggested name for an idea', () => {
    const p = findPreset('webhook')
    expect(p.runtime).toBe('node')
    expect(presetDefaults(p)).toMatchObject({
      source: 'repo',
      name: 'webhook-receiver',
      idleChoice: '5',
      port: '8080',
    })
  })

  it('uses the runtime port from the API when given', () => {
    expect(presetDefaults(findPreset('go'), 3000).port).toBe('3000')
  })

  it('switches to image source for the image card', () => {
    expect(presetDefaults(findPreset('image')).source).toBe('image')
  })
})

describe('name suggestion', () => {
  it('kebab-cases awkward input', () => {
    expect(toKebab('My_Cool App!.git')).toBe('my-cool-app')
  })

  it('derives from https and ssh repo urls', () => {
    expect(nameFromRepoUrl('https://github.com/you/Hello_World.git')).toBe(
      'hello-world',
    )
    expect(nameFromRepoUrl('git@github.com:you/api/')).toBe('api')
  })

  it('derives from an image reference without registry or tag', () => {
    expect(nameFromImage('ghcr.io/you/Hello:1.0')).toBe('hello')
    expect(nameFromImage('nginx@sha256:abc')).toBe('nginx')
  })

  it('falls back to the preset name when the source gives nothing', () => {
    expect(
      suggestName({
        source: 'repo',
        repoUrl: '',
        image: '',
        presetName: 'url-shortener',
      }),
    ).toBe('url-shortener')
  })
})

describe('url preview', () => {
  const domains = [
    domain({ domain: 'web.1-2-3-4.sslip.io', automatic: true }),
    domain({ domain: 'example.com' }),
  ]

  it('derives the generated host from an automatic domain', () => {
    expect(functionHost('hello', domains)).toBe('hello.1-2-3-4.sslip.io')
    expect(functionHost('hello', [])).toBeNull()
  })

  it('builds both urls and normalizes the path', () => {
    expect(
      previewUrls({
        name: 'hello',
        routeDomain: 'example.com',
        routePath: 'hi/',
        domains,
      }),
    ).toEqual({
      functionUrl: 'https://hello.1-2-3-4.sslip.io',
      routeUrl: 'https://example.com/hi',
    })
    expect(normalizePath('')).toBe('')
  })

  it('omits the route url without a domain or path', () => {
    expect(
      previewUrls({ name: 'a', routeDomain: '', routePath: '/x', domains })
        .routeUrl,
    ).toBeNull()
  })

  it('lists only domains attached to an app', () => {
    const list = [
      domain({ domain: 'a.com' }),
      domain({ domain: 'b.com', service_name: '', kind: 'standalone' }),
    ]
    expect(routableDomains(list).map((d) => d.domain)).toEqual(['a.com'])
  })
})

describe('validation messages', () => {
  it('accepts a complete form', () => {
    expect(validate(valid)).toEqual({})
  })

  it('asks for the missing repository and name', () => {
    expect(validate({ ...valid, repoUrl: '', name: '' })).toEqual({
      repoUrl: 'validation.repoRequired',
      name: 'validation.nameRequired',
    })
  })

  it('rejects a bad url, name, port and custom idle', () => {
    const e = validate({
      ...valid,
      repoUrl: 'github.com/x',
      name: 'Bad Name',
      port: '70000',
      idleChoice: 'custom',
      customIdle: '1',
    })
    expect(e).toEqual({
      repoUrl: 'validation.repoInvalid',
      name: 'validation.nameInvalid',
      port: 'validation.portInvalid',
      customIdle: 'validation.idleInvalid',
    })
  })

  it('flags a taken name and a path without a domain', () => {
    const e = validate({ ...valid, routePath: '/hello' }, true)
    expect(e.name).toBe('validation.nameTaken')
    expect(e.routeDomain).toBe('validation.domainRequired')
  })

  it('validates the image field only for image sources', () => {
    expect(validate({ ...valid, source: 'image', image: '' }).image).toBe(
      'validation.imageRequired',
    )
    expect(
      validate({ ...valid, source: 'image', image: 'nginx:1' }).image,
    ).toBeUndefined()
  })
})
