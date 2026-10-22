import { describe, expect, it } from 'vitest'
import { databaseHealth } from './databaseHealth'
import type { ReconcileCondition } from '../types/deploy'

const cond = (Status: ReconcileCondition['Status']): ReconcileCondition => ({
  Type: 'Ready',
  Status,
  Reason: 'x',
  Message: '',
  LastTransitionTime: '',
})

describe('databaseHealth', () => {
  it('maps provisioning and stopped states', () => {
    expect(databaseHealth({ provision_state: 'pending' }, []).state).toBe(
      'settingUp',
    )
    expect(databaseHealth({ provision_state: 'failed' }, []).state).toBe(
      'attention',
    )
    expect(databaseHealth({ suspended: true }, [cond('True')])).toEqual({
      state: 'stopped',
      action: 'start',
    })
  })
  it('reads conditions', () => {
    expect(databaseHealth({}, [cond('True')])).toEqual({
      state: 'healthy',
      action: 'connect',
    })
    expect(databaseHealth({}, [cond('True'), cond('False')]).state).toBe(
      'attention',
    )
    expect(databaseHealth({}, [cond('Unknown')]).state).toBe('settingUp')
    expect(databaseHealth({}, []).state).toBe('settingUp')
  })
})
