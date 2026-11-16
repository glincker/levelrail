import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import cloudEn from '../locales/en/cloudProvider.json'
import {
  CLOUD_PROVIDERS,
  CLOUD_PROVIDER_IDS,
  cloudProviderFor,
  cloudProviderPath,
} from './cloudProviderBrand'

describe('cloud provider presentation', () => {
  it.each(CLOUD_PROVIDER_IDS)('%s has config, copy and a path', (id) => {
    expect(cloudProviderFor(id)).toBe(CLOUD_PROVIDERS[id])
    expect(cloudEn.providers[id].terms.servers).toBeTruthy()
    expect(cloudProviderPath(id)).toBe(`/integrations/cloud/${id}`)
    expect(CLOUD_PROVIDERS[id].features).toContain('connection')
  })

  it('rejects an unknown provider', () => {
    expect(cloudProviderFor('nope')).toBeNull()
  })

  it('registers the landing route when the generated tree exists', () => {
    const tree = join(__dirname, '..', 'routeTree.gen.ts')
    if (!existsSync(tree)) return
    expect(readFileSync(tree, 'utf8')).toContain(
      '/integrations/cloud/$provider',
    )
  })
})
