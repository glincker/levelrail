import { describe, expect, it } from 'vitest'
import {
  brandForRegistryHost,
  isKnownRegistryRef,
  registryHostOf,
} from './registryRef'

describe('registryRef', () => {
  it('reads the host of a reference', () => {
    expect(registryHostOf('ghcr.io/org/app:1.2')).toBe('ghcr.io')
    expect(registryHostOf('registry.example.com:5000/team/app')).toBe(
      'registry.example.com',
    )
    expect(registryHostOf('nginx')).toBeNull()
    expect(registryHostOf('library/nginx')).toBeNull()
  })

  it('treats well-known registries as images without a prefix', () => {
    expect(isKnownRegistryRef('quay.io/prometheus/prometheus')).toBe(true)
    expect(
      isKnownRegistryRef('123456789012.dkr.ecr.us-east-1.amazonaws.com/app'),
    ).toBe(true)
    expect(isKnownRegistryRef('europe-docker.pkg.dev/p/r/app')).toBe(true)
    expect(isKnownRegistryRef('example.com/some/path')).toBe(false)
  })

  it('picks a brand mark per host', () => {
    expect(brandForRegistryHost('ghcr.io')).toBe('github')
    expect(brandForRegistryHost('quay.io')).toBe('quay')
    expect(brandForRegistryHost('registry.gitlab.com')).toBe('gitlab')
    expect(brandForRegistryHost('foo.azurecr.io')).toBe(
      'azure-container-registry',
    )
    expect(brandForRegistryHost('public.ecr.aws')).toBe('ecr')
    expect(brandForRegistryHost('docker.io')).toBe('docker')
    expect(brandForRegistryHost('registry.example.com')).toBeNull()
  })
})
