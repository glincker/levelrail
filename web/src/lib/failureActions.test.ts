import { describe, expect, it } from 'vitest'
import { failureActionFor, isUnknownFailure } from './failureActions'

const KNOWN_CODES = [
  'dockerfile_error',
  'dependency_install_failed',
  'compile_error',
  'build_config_error',
  'build_out_of_memory',
  'build_timeout',
  'image_pull_failed',
  'registry_auth_failed',
  'platform_mismatch',
  'lockfile_out_of_sync',
  'port_not_listening',
  'health_check_failed',
  'container_crashed',
  'oom_killed',
  'missing_env',
  'registry_push_failed',
  'disk_full',
  'image_corrupt',
  'freeze_window_hold',
  'approval_pending',
  'scan_gate_blocked',
  'rollback_target_gone',
  'docker_unreachable',
  'git_host_unreachable',
  'source_not_cached',
]

describe('failureActionFor', () => {
  it.each(KNOWN_CODES)('maps %s to a deep link', (code) => {
    const action = failureActionFor(code, 'web', 'dep_1')
    expect(action).not.toBeNull()
    expect(action?.href.startsWith('/')).toBe(true)
    expect(isUnknownFailure(code)).toBe(false)
  })

  it('sends an unknown failure to the deploy log', () => {
    const action = failureActionFor('unknown', 'web', 'dep_1')
    expect(action?.href).toBe('/apps/web/deploys/dep_1/logs')
    expect(isUnknownFailure('unknown')).toBe(true)
  })

  it('returns nothing without an app', () => {
    expect(failureActionFor('missing_env', '', 'dep_1')).toBeNull()
  })

  it('encodes the app name', () => {
    expect(failureActionFor('missing_env', 'a b', 'd')?.href).toBe(
      '/apps/a%20b/environment',
    )
  })
})
