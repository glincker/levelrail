import type { Query, QueryClient } from '@tanstack/react-query'

// Literal keys keep the list query modules out of the shell chunk; the test pins them to the real key factories.
export const CHANGE_TOPIC_KEYS = {
  apps: [
    ['apps', 'list'],
    ['apps', 'pages'],
    ['apps', 'summary'],
  ],
  nodes: [['nodes', 'list']],
  domains: [['domains']],
  alerts: [],
} as const satisfies Record<string, readonly (readonly string[])[]>

export type ChangeTopic = keyof typeof CHANGE_TOPIC_KEYS

export function isChangeTopic(v: unknown): v is ChangeTopic {
  return typeof v === 'string' && Object.hasOwn(CHANGE_TOPIC_KEYS, v)
}

// Alert rules live under each app's detail key, so they match by shape.
export function isAlertRuleQuery(q: Query): boolean {
  const k = q.queryKey
  return k[0] === 'apps' && k[1] === 'detail' && k[3] === 'alerts'
}

export function invalidateChangeTopic(
  qc: QueryClient,
  topic: ChangeTopic,
): void {
  for (const key of CHANGE_TOPIC_KEYS[topic]) {
    void qc.invalidateQueries({ queryKey: key })
  }
  if (topic === 'alerts') {
    void qc.invalidateQueries({ predicate: isAlertRuleQuery })
  }
}
