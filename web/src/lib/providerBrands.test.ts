import { describe, expect, it } from 'vitest'
import { isBrandIconName } from '../components/BrandIcon'
import {
  backupTargetBrand,
  dnsProviderBrand,
  nodeProviderBrand,
  registryBrandForHost,
  storagePresetBrand,
} from './providerBrands'

describe('providerBrands', () => {
  it.each([
    ['cloudflare', 'cloudflare'],
    ['route53', 'route53'],
    ['none', ''],
  ])('dns %s', (provider, want) => {
    expect(dnsProviderBrand(provider)).toBe(want)
  })

  it.each([
    ['hetzner', 'hetzner'],
    ['digitalocean', 'digitalocean'],
    ['gcp', 'google-cloud'],
    ['azure', 'azure'],
    ['unknown-cloud', ''],
  ])('node provider %s', (provider, want) => {
    expect(nodeProviderBrand(provider)).toBe(want)
  })

  it.each([
    ['docker.io', 'docker'],
    ['ghcr.io', 'github'],
    ['gcr.io', 'google-artifact-registry'],
    ['us-docker.pkg.dev/proj/repo', 'google-artifact-registry'],
    ['123456789012.dkr.ecr.us-east-1.amazonaws.com', 'ecr'],
    ['myregistry.azurecr.io', 'azure'],
    ['registry.gitlab.com', 'gitlab'],
    ['harbor.example.com', 'harbor'],
    ['registry.example.com', ''],
    ['ghcr.io.evil.example', ''],
    ['', ''],
  ])('registry host %s', (host, want) => {
    expect(registryBrandForHost(host)).toBe(want)
  })

  it.each([
    ['aws', undefined, 's3'],
    ['r2', undefined, 'cloudflare'],
    ['custom', 'https://s3.us-west-004.backblazeb2.com', 'backblaze'],
    ['custom', 'http://minio.local:9000', 'minio'],
    ['custom', 'https://s3.example.com', ''],
    ['custom', undefined, ''],
  ] as const)('backup %s %s', (provider, endpoint, want) => {
    expect(backupTargetBrand(provider, endpoint)).toBe(want)
  })

  it('only returns keys BrandIcon knows', () => {
    const names = [
      dnsProviderBrand('cloudflare'),
      dnsProviderBrand('route53'),
      nodeProviderBrand('hetzner'),
      nodeProviderBrand('digitalocean'),
      nodeProviderBrand('aws'),
      nodeProviderBrand('azure'),
      nodeProviderBrand('gcp'),
      nodeProviderBrand('linode'),
      nodeProviderBrand('akamai'),
      nodeProviderBrand('oracle'),
      registryBrandForHost('docker.io'),
      registryBrandForHost('ghcr.io'),
      registryBrandForHost('gcr.io'),
      registryBrandForHost('a.dkr.ecr.us-east-1.amazonaws.com'),
      registryBrandForHost('x.azurecr.io'),
      registryBrandForHost('registry.gitlab.com'),
      registryBrandForHost('registry.digitalocean.com'),
      registryBrandForHost('x.jfrog.io'),
      registryBrandForHost('harbor.example.com'),
      backupTargetBrand('aws'),
      backupTargetBrand('custom', 'https://storage.googleapis.com'),
      storagePresetBrand('b2'),
      storagePresetBrand('minio'),
    ]
    for (const name of names) {
      expect(isBrandIconName(name), name).toBe(true)
    }
  })
})
