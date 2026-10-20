import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import type { AttentionItem } from './attention'
import { attentionToSuggestions, seedMessageFor } from './fleetSuggestions'
import dashboardEn from '../locales/en/dashboard.json'

function item(
  over: Partial<AttentionItem> & Pick<AttentionItem, 'id' | 'target'>,
): AttentionItem {
  return { severity: 'critical', title: 't', detail: 'd', ...over }
}

const testI18n = i18next.createInstance()
void testI18n.init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboard'],
  defaultNS: 'dashboard',
  resources: { en: { dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})
const t = testI18n.getFixedT('en', 'dashboard')

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
        ['Ask AI', 'ask-ai'],
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
        ['Ask AI', 'ask-ai'],
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
        ['Ask AI', 'ask-ai'],
      ],
    },
    {
      name: 'offline node',
      input: item({ id: 'node:n1', target: { kind: 'node', id: 'n1' } }),
      tone: 'danger',
      title: 't',
      actions: [
        ['Open node', 'node'],
        ['Ask AI', 'ask-ai'],
      ],
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
      actions: [
        ['Review certificate', 'domains'],
        ['Ask AI', 'ask-ai'],
      ],
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
      actions: [
        ['Clean up Docker', 'cleanup'],
        ['Ask AI', 'ask-ai'],
      ],
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
      actions: [
        ['Fix', 'system'],
        ['Ask AI', 'ask-ai'],
      ],
    },
  ]

  it.each(cases)('maps $name', ({ input, tone, title, actions }) => {
    const [spec] = attentionToSuggestions([input], t)
    expect(spec?.tone).toBe(tone)
    expect(spec?.title).toBe(title)
    expect(spec?.actions.map((a) => [a.label, a.spec.kind])).toEqual(actions)
    expect(spec?.actions[0]?.primary).toBe(true)
    expect(spec?.actions.at(-1)?.primary).toBe(false)
  })

  it('returns nothing for no items', () => {
    expect(attentionToSuggestions([], t)).toEqual([])
  })
})

describe('seedMessageFor', () => {
  it('includes the app name for an app target', () => {
    const msg = seedMessageFor(
      item({
        id: 'app:web',
        title: 'web',
        detail: 'Exited with code 1',
        target: { kind: 'app', name: 'web' },
      }),
    )
    expect(msg).toBe(
      'Help me fix this: web is failing. Exited with code 1. Target: web.',
    )
  })

  it('includes the app name for a deploy target', () => {
    const msg = seedMessageFor(
      item({
        id: 'deploy:api',
        title: 'Deploy of api failed',
        detail: 'Image pull failed',
        target: { kind: 'deploy', app: 'api', image: 'api:2' },
      }),
    )
    expect(msg).toContain('Target: api.')
  })

  it('omits a target line for a domain or system item', () => {
    const msg = seedMessageFor(
      item({
        id: 'cert:a.com',
        title: 'Certificate for a.com expires soon',
        detail: 'Expires in 3 days',
        target: { kind: 'domain' },
      }),
    )
    expect(msg).toBe(
      'Help me fix this: Certificate for a.com expires soon. Expires in 3 days.',
    )
  })
})
