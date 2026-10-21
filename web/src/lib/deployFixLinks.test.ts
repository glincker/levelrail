import { describe, expect, it } from 'vitest'
import { deployFixLinks } from './deployFixLinks'

describe('deployFixLinks', () => {
  it('maps env causes to the environment page', () => {
    expect(deployFixLinks('build', 'missing-env-var')[0]?.to).toBe(
      '/apps/$name/environment',
    )
  })

  it('maps build path causes to the source page', () => {
    expect(deployFixLinks('build', 'dockerfile-path')[0]?.to).toBe(
      '/apps/$name/source',
    )
  })

  it('maps port and health causes to deploy settings', () => {
    expect(deployFixLinks('rollout', 'port-mismatch')[0]?.to).toBe(
      '/apps/$name/deploy-settings',
    )
    expect(deployFixLinks('health-check', 'health-check-timeout')[0]?.to).toBe(
      '/apps/$name/deploy-settings',
    )
  })

  it('falls back to the failing stage when no cause matched', () => {
    expect(deployFixLinks('build')[0]?.to).toBe('/apps/$name/source')
    expect(deployFixLinks('rollout', 'unknown-id')[0]?.to).toBe(
      '/apps/$name/deploy-settings',
    )
  })
})
