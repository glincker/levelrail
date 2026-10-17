import { useNavigate, useSearch } from '@tanstack/react-router'
import type { FwField, FwRange } from '../../queries/firewallEdge'
import type { RuleSeed } from './useFirewallActions'

export interface FirewallSearch {
  scope?: string
  range?: FwRange
}

export interface RuleSearch extends FirewallSearch {
  field?: FwField
  value?: string
}

const RANGES: FwRange[] = ['1h', '6h', '24h']
const FIELDS: FwField[] = [
  'path',
  'host',
  'method',
  'header',
  'query',
  'user_agent',
  'ip',
  'country',
]

// Plain functions rather than zod so validateSearch stays out of the eagerly loaded bundle.
export function validateFirewallSearch(
  search: Record<string, unknown>,
): FirewallSearch {
  const scope =
    typeof search.scope === 'string' && search.scope !== ''
      ? search.scope
      : undefined
  const range = RANGES.find((r) => r === search.range)
  return { scope, range }
}

export function validateRuleSearch(
  search: Record<string, unknown>,
): RuleSearch {
  const field = FIELDS.find((f) => f === search.field)
  const value =
    typeof search.value === 'string' && search.value !== ''
      ? search.value
      : undefined
  return {
    ...validateFirewallSearch(search),
    field: field && value ? field : undefined,
    value: field && value ? value : undefined,
  }
}

/** useFirewallNav reads the scope and range from the URL and navigates between firewall views keeping them. */
export function useFirewallNav() {
  const navigate = useNavigate()
  const search: RuleSearch = useSearch({ strict: false })
  const scope = search.scope ?? ''
  const range: FwRange = search.range ?? '1h'
  const keep = { scope: scope || undefined, range }

  return {
    scope,
    range,
    seed:
      search.field && search.value
        ? ({ field: search.field, value: search.value } satisfies RuleSeed)
        : undefined,
    setScope: (next: string) =>
      void navigate({
        to: '.',
        search: (prev: FirewallSearch) => ({
          ...prev,
          scope: next || undefined,
        }),
        replace: true,
      }),
    setRange: (next: FwRange) =>
      void navigate({
        to: '.',
        search: (prev: FirewallSearch) => ({ ...prev, range: next }),
        replace: true,
      }),
    openRules: () => void navigate({ to: '/firewall/rules', search: keep }),
    newRule: () => void navigate({ to: '/firewall/rules/new', search: keep }),
    editRule: (id: string) =>
      void navigate({
        to: '/firewall/rules/$id',
        params: { id },
        search: keep,
      }),
    createRuleFrom: (seed: RuleSeed) =>
      void navigate({
        to: '/firewall/rules/new',
        search: { ...keep, field: seed.field, value: seed.value },
      }),
  }
}
