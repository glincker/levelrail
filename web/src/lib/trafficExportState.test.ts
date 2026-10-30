import { describe, expect, it } from 'vitest'
import { chipState, hasStoredCredential } from './trafficExportState'
import type {
  TrafficExportSink,
  TrafficExportSinkStatus,
} from '../types/trafficExport'

const sink: TrafficExportSink = {
  id: 'a',
  name: 'a',
  kind: 'webhook',
  enabled: true,
  fields: [],
  ip_truncate: true,
  sample: 1,
  identity_enabled: false,
}

const live: TrafficExportSinkStatus = {
  id: 'a',
  name: 'a',
  kind: 'webhook',
  enabled: true,
  active: true,
  last_success: '',
  queued: 0,
  sent: 0,
  dropped: 0,
  failed: 0,
  sampled_out: 0,
}

describe('chipState', () => {
  it.each([
    ['paused sink', { ...sink, enabled: false }, live, 'disabled'],
    ['no status yet', sink, undefined, 'idle'],
    ['active and healthy', sink, live, 'active'],
    [
      'active with a failed delivery',
      sink,
      { ...live, last_error_kind: 'timeout' },
      'failing',
    ],
    [
      'rejected config',
      sink,
      { ...live, active: false, last_error_kind: 'config' },
      'invalid',
    ],
    ['not running, no error', sink, { ...live, active: false }, 'idle'],
  ] as const)('%s', (_name, s, st, want) => {
    expect(chipState(s, st)).toBe(want)
  })
})

describe('hasStoredCredential', () => {
  it('reads the slot that belongs to the kind', () => {
    expect(
      hasStoredCredential({ ...sink, kind: 'posthog', token_set: true }),
    ).toBe(true)
    expect(
      hasStoredCredential({ ...sink, kind: 'posthog', url_set: true }),
    ).toBe(false)
    expect(hasStoredCredential({ ...sink, url_set: true })).toBe(true)
  })
})
