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

  it('prefers the control plane fix kinds and dedupes their links', () => {
    const fix = (
      kind: 'set_secret' | 'edit_source' | 'rerun' | 'free_disk',
    ) => ({
      code: kind,
      kind,
      hint: 'h',
      match: 'm',
    })
    const links = deployFixLinks('build', 'dockerfile-path', [
      fix('set_secret'),
      fix('edit_source'),
      fix('free_disk'),
      fix('rerun'),
    ])
    expect(links.map((l) => l.to)).toEqual([
      '/apps/$name/environment',
      '/apps/$name/source',
    ])
  })

  it('falls back to the cause rules when the typed fixes need no setting change', () => {
    const rerun = { code: 'r', kind: 'rerun' as const, hint: 'h', match: 'm' }
    expect(deployFixLinks('build', 'missing-env-var', [rerun])[0]?.to).toBe(
      '/apps/$name/environment',
    )
  })
})
