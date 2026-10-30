import { describe, expect, it } from 'vitest'
import type { DnsDetection } from '@/queries/domainDns'
import type { MailPlan, MailPlanChange } from '@/queries/mailDns'
import {
  apexSuggestion,
  detectState,
  groupChanges,
  liveCount,
  normalizeDomain,
  primaryActionOf,
  rowStatus,
  tableText,
  zoneFileText,
} from './dnsSetup'

const ch = (
  kind: string,
  action: MailPlanChange['action'],
  extra: Partial<MailPlanChange> = {},
): MailPlanChange => ({
  action,
  kind,
  set: {
    name: `${kind.toLowerCase()}.example.com`,
    type: kind === 'MX' ? 'MX' : 'TXT',
    ttl: 300,
    values: ['10 mail.example.com'],
  },
  ...extra,
})
const plan = (changes: MailPlanChange[]): MailPlan => ({
  domain: 'example.com',
  hash: 'h',
  changes,
  summary: {},
  blocked: false,
})
const det = (o: Partial<DnsDetection>): DnsDetection => ({
  domain: 'example.com',
  apex: 'example.com',
  zone: 'example.com',
  nameservers: [],
  provider: 'cloudflare',
  domain_connect: { supported: false },
  connected: true,
  connectable: true,
  can_issue_certificates: false,
  hint: '',
  ...o,
})

describe('domain input', () => {
  it('strips scheme, path, www and port', () => {
    expect(normalizeDomain('https://www.Example.com/path?x=1')).toBe(
      'example.com',
    )
    expect(normalizeDomain('example.com:8080')).toBe('example.com')
    expect(normalizeDomain('nope')).toBe('')
  })
  it('offers the apex for mail. and smtp. hosts only', () => {
    expect(apexSuggestion('mail.glinr.com')).toEqual({
      apex: 'glinr.com',
      host: 'mail.glinr.com',
    })
    expect(apexSuggestion('smtp.glinr.com')?.apex).toBe('glinr.com')
    expect(apexSuggestion('glinr.com')).toBeNull()
    expect(apexSuggestion('app.glinr.com')).toBeNull()
  })
})

describe('detection and primary action', () => {
  it('maps the three detection states', () => {
    expect(detectState(det({}))).toBe('connected')
    expect(detectState(det({ connected: false }))).toBe('notConnected')
    expect(detectState(det({ provider: '' }))).toBe('unknown')
    expect(detectState(undefined)).toBe('unknown')
  })
  it('picks the one primary action', () => {
    expect(primaryActionOf('connected', 3)).toBe('add')
    expect(primaryActionOf('connected', 0)).toBe('check')
    expect(primaryActionOf('notConnected', 3, true)).toBe('connect')
    expect(primaryActionOf('notConnected', 3, false)).toBe('copy')
    expect(primaryActionOf('unknown', 3)).toBe('copy')
  })
})

describe('rows and grouping', () => {
  it('derives row states', () => {
    expect(rowStatus(ch('SPF', 'unchanged'))).toBe('live')
    expect(rowStatus(ch('SPF', 'add'))).toBe('waiting')
    expect(rowStatus(ch('SPF', 'replace'))).toBe('different')
    expect(rowStatus(ch('SPF', 'refused'))).toBe('conflict')
    expect(
      rowStatus(
        ch('SPF', 'refused', {
          before: { name: 'x', type: 'TXT', ttl: 1, values: ['v'] },
        }),
      ),
    ).toBe('different')
    expect(liveCount(['live', 'unknown', 'live'])).toBe(2)
  })
  it('never reports unknown as live from the public check', () => {
    expect(rowStatus(ch('SPF', 'add'), { domain: 'd', items: [] })).toBe(
      'unknown',
    )
    expect(
      rowStatus(ch('SPF', 'add'), {
        domain: 'd',
        items: [{ id: 'spf', status: 'pass', message: '' }],
      }),
    ).toBe('live')
  })
  it('groups mail records', () => {
    const g = groupChanges(
      'mail',
      plan([
        ch('MX', 'add'),
        ch('SPF', 'add'),
        ch('DKIM', 'add'),
        ch('MTASTS', 'add'),
      ]),
    )
    expect(g.map((x) => [x.group, x.changes.length])).toEqual([
      ['receive', 1],
      ['trust', 2],
      ['optional', 1],
    ])
  })
})

describe('copy outputs', () => {
  const p = plan([ch('MX', 'add'), ch('SPF', 'add')])
  it('writes a zone-file block', () => {
    const lines = zoneFileText(p).split('\n')
    expect(lines[0]).toBe('mx.example.com.\t300\tIN\tMX\t10 mail.example.com')
    expect(lines[1]).toContain('IN\tTXT\t"10 mail.example.com"')
  })
  it('writes a table with a header', () => {
    expect(tableText(p).split('\n')[0]).toBe('Type\tName\tValue\tTTL')
  })
})
