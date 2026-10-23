import type { ReconcileCondition } from '../types/deploy'
import type { DatabaseResource } from '../types/databaseDetail'

export type HealthState = 'healthy' | 'settingUp' | 'stopped' | 'attention'
export type PrimaryAction = 'connect' | 'start' | 'logs' | null

export interface DatabaseHealth {
  state: HealthState
  action: PrimaryAction
}

/** One operator-facing state derived only from fields the API already returns. */
export function databaseHealth(
  db: Pick<DatabaseResource, 'provision_state' | 'suspended'>,
  conditions: ReconcileCondition[],
): DatabaseHealth {
  if (db.provision_state === 'failed') {
    return { state: 'attention', action: 'logs' }
  }
  if (db.provision_state === 'pending') {
    return { state: 'settingUp', action: null }
  }
  if (db.suspended) return { state: 'stopped', action: 'start' }
  if (conditions.some((c) => c.Status === 'False')) {
    return { state: 'attention', action: 'logs' }
  }
  if (conditions.length === 0 || conditions.some((c) => c.Status !== 'True')) {
    return { state: 'settingUp', action: null }
  }
  return { state: 'healthy', action: 'connect' }
}
