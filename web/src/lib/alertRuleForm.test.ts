import { describe, expect, it } from 'vitest'
import {
  ALERT_RULE_DEFAULT_VALUES,
  alertRuleFormSchema,
  buildAlertRuleRequest,
  type AlertRuleFormOutput,
} from './alertRuleForm'

function values(patch: Partial<AlertRuleFormOutput>): AlertRuleFormOutput {
  return alertRuleFormSchema.parse({
    ...ALERT_RULE_DEFAULT_VALUES,
    name: ' rule ',
    metric: 'cpu',
    ...patch,
  })
}

describe('buildAlertRuleRequest', () => {
  it('sends metric fields for a threshold rule and trims the name', () => {
    const req = buildAlertRuleRequest(values({}), 'web')
    expect(req).toMatchObject({
      name: 'rule',
      kind: 'threshold',
      metric: 'cpu',
      comparator: '>',
      threshold: 80,
      for_duration: '2m',
    })
  })

  it('sets the app as service for a volume backup rule', () => {
    const req = buildAlertRuleRequest(
      values({
        kind: 'backup_missing',
        backupResourceKind: 'volume',
        backupVolumeName: 'data',
      }),
      'web',
    )
    expect(req.backup_service_name).toBe('web')
    expect(req.backup_volume_name).toBe('data')
    expect(req.metric).toBeUndefined()
  })

  it('carries extra fields through for edits', () => {
    const req = buildAlertRuleRequest(values({}), 'web', {
      severity: 'critical',
    })
    expect(req.severity).toBe('critical')
  })
})

describe('alertRuleFormSchema', () => {
  it('requires a metric for threshold rules', () => {
    const r = alertRuleFormSchema.safeParse({
      ...ALERT_RULE_DEFAULT_VALUES,
      name: 'x',
    })
    expect(r.success).toBe(false)
  })

  it('rejects a malformed duration', () => {
    const r = alertRuleFormSchema.safeParse({
      ...ALERT_RULE_DEFAULT_VALUES,
      name: 'x',
      metric: 'cpu',
      forDuration: 'soon',
    })
    expect(r.success).toBe(false)
  })
})
