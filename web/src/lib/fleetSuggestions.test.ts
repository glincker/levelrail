import { describe, expect, it } from 'vitest'
import type { AttentionItem } from './attention'
import { attentionToSuggestions } from './fleetSuggestions'

function item(
  over: Partial<AttentionItem> & Pick<AttentionItem, 'id' | 'target'>,
): AttentionItem {
  return { severity: 'critical', title: 't', detail: 'd', ...over }
}

describe('attentionToSuggestions', () => {
  const cases: {
    name: string
    input: AttentionItem
    tone: string
    title: string
    actions: [string, string][]
  }[] = [
    {
      name: 'failing app',
      input: item({
        id: 'app:web',
        title: 'web',
        target: { kind: 'app', name: 'web' },
      }),
      tone: 'danger',
      title: 'web is failing',
      actions: [
        ['Restart', 'restart'],
        ['Open logs', 'logs'],
      ],
    },
    {
      name: 'failed deploy with a last good image',
      input: item({
        id: 'deploy:web',
        title: 'Deploy of web failed',
        target: {
          kind: 'deploy',
          app: 'web',
          image: 'web:2',
          lastGoodImage: 'web:1',
        },
      }),
      tone: 'danger',
      title: 'Deploy of web failed',
      actions: [
        ['Roll back', 'redeploy'],
        ['Open logs', 'logs'],
      ],
    },
    {
      name: 'failed deploy without a last good image',
      input: item({
        id: 'deploy:api',
        target: { kind: 'deploy', app: 'api', image: 'api:2' },
      }),
      tone: 'danger',
      title: 't',
      actions: [
        ['Redeploy', 'redeploy'],
        ['Open logs', 'logs'],
      ],
    },
    {
      name: 'offline node',
      input: item({ id: 'node:n1', target: { kind: 'node', id: 'n1' } }),
      tone: 'danger',
      title: 't',
      actions: [['Open node', 'node']],
    },
    {
      name: 'expiring certificate',
      input: item({
        id: 'cert:a.com',
        severity: 'warning',
        target: { kind: 'domain' },
      }),
      tone: 'warning',
      title: 't',
      actions: [['Review certificate', 'domains']],
    },
    {
      name: 'low disk',
      input: item({
        id: 'disk',
        severity: 'warning',
        target: { kind: 'system' },
      }),
      tone: 'warning',
      title: 't',
      actions: [['Clean up Docker', 'cleanup']],
    },
    {
      name: 'doctor finding',
      input: item({
        id: 'doctor:x',
        severity: 'warning',
        target: { kind: 'system' },
      }),
      tone: 'warning',
      title: 't',
      actions: [['Fix', 'system']],
    },
  ]

  it.each(cases)('maps $name', ({ input, tone, title, actions }) => {
    const [spec] = attentionToSuggestions([input])
    expect(spec?.tone).toBe(tone)
    expect(spec?.title).toBe(title)
    expect(spec?.actions.map((a) => [a.label, a.spec.kind])).toEqual(actions)
    expect(spec?.actions[0]?.primary).toBe(true)
  })

  it('returns nothing for no items', () => {
    expect(attentionToSuggestions([])).toEqual([])
  })
})
