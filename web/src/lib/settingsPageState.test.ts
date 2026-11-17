import { describe, expect, it } from 'vitest'
import {
  showHeartbeat,
  showRegistryExtras,
  showRevokedTokens,
  statusPageTabs,
} from './settingsPageState'

describe('settings page state rules', () => {
  it('shows no tabs when the status page is off', () => {
    expect(statusPageTabs({ enabled: false, componentCount: 3 })).toEqual([])
  })
  it('shows only settings and components when there are no components', () => {
    expect(statusPageTabs({ enabled: true, componentCount: 0 })).toEqual([
      'settings',
      'components',
    ])
  })
  it('shows all four tabs with components', () => {
    expect(statusPageTabs({ enabled: true, componentCount: 1 })).toHaveLength(4)
  })
  it('shows revoked tokens and heartbeat only with something to act on', () => {
    expect(showRevokedTokens(0)).toBe(false)
    expect(showRevokedTokens(2)).toBe(true)
    expect(showHeartbeat(0)).toBe(false)
    expect(showHeartbeat(1)).toBe(true)
  })
  it('shows registry extras on any signal and never while loading', () => {
    const base = {
      loading: false,
      credentialCount: 0,
      builtInEnabled: false,
      packageRegistryCount: 0,
    }
    expect(showRegistryExtras(base)).toBe(false)
    expect(showRegistryExtras({ ...base, credentialCount: 1 })).toBe(true)
    expect(showRegistryExtras({ ...base, builtInEnabled: true })).toBe(true)
    expect(showRegistryExtras({ ...base, packageRegistryCount: 1 })).toBe(true)
    expect(
      showRegistryExtras({ ...base, loading: true, credentialCount: 1 }),
    ).toBe(false)
  })
})
