import { describe, expect, it } from 'vitest'
import {
  ALERT_RULE_DEFAULT_VALUES,
  alertRuleFormSchema,
  buildAlertRuleRequest,
  type AlertRuleFormOutput,
} from './alertRuleForm'
import { domainFromResourceId } from './alertTrafficKinds'

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

describe('traffic alert kinds', () => {
  const drop = {
    kind: 'domain_traffic_drop' as const,
    forDuration: '1h',
    threshold: 50,
    restartCountThreshold: 20,
  }

  it('maps the drop form to window, percent and minimum requests', () => {
    const req = buildAlertRuleRequest(values(drop), 'web')
    expect(req).toMatchObject({
      kind: 'domain_traffic_drop',
      resource_id: 'service:web',
      for_duration: '1h',
      threshold: 50,
      restart_count_threshold: 20,
    })
    expect(req.metric).toBeUndefined()
  })

  it('scopes to one domain when chosen', () => {
    const req = buildAlertRuleRequest(
      values({ ...drop, trafficDomain: 'shop.example.com' }),
      'web',
    )
    expect(req.resource_id).toBe('domain:shop.example.com')
  })

  it('rejects out of range window, percent and minimum', () => {
    const bad = alertRuleFormSchema.safeParse({
      ...ALERT_RULE_DEFAULT_VALUES,
      name: 'x',
      ...drop,
      forDuration: '25h',
      threshold: 101,
      restartCountThreshold: 0,
    })
    expect(bad.success).toBe(false)
    const paths = bad.success ? [] : bad.error.issues.map((i) => i.path[0])
    expect(paths).toEqual(
      expect.arrayContaining([
        'forDuration',
        'threshold',
        'restartCountThreshold',
      ]),
    )
  })

  it('accepts the 24h and 0 percent edges', () => {
    expect(
      alertRuleFormSchema.safeParse({
        ...ALERT_RULE_DEFAULT_VALUES,
        name: 'x',
        ...drop,
        forDuration: '24h',
        threshold: 0,
      }).success,
    ).toBe(true)
  })

  it('sends only days left for the parked certificate kind', () => {
    const req = buildAlertRuleRequest(
      values({ kind: 'parked_domain_cert_expiry', threshold: 14 }),
      'web',
    )
    expect(req.threshold).toBe(14)
    expect(req.resource_id).toBeUndefined()
    expect(req.for_duration).toBeUndefined()
  })

  it('rejects a non whole number of days', () => {
    expect(
      alertRuleFormSchema.safeParse({
        ...ALERT_RULE_DEFAULT_VALUES,
        name: 'x',
        kind: 'parked_domain_cert_expiry',
        threshold: 0.5,
      }).success,
    ).toBe(false)
  })

  it('reads the domain back from a saved resource id', () => {
    expect(domainFromResourceId('domain:a.io')).toBe('a.io')
    expect(domainFromResourceId('service:web')).toBe('')
  })
})
