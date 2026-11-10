import type { FwRule } from '../../queries/firewallEdge'
import { conditionText } from './firewallRuleModel'

export type RuleSort = 'default' | 'name' | 'hits' | 'recent'

export const RULE_SORTS: readonly RuleSort[] = [
  'default',
  'name',
  'hits',
  'recent',
]

/** ruleSearchText is the lowercase haystack a search query is matched against. */
function ruleSearchText(r: FwRule): string {
  return [
    r.name,
    r.note ?? '',
    r.scope ?? '',
    ...r.conditions.map(conditionText),
  ]
    .join(' ')
    .toLowerCase()
}

/** filterRules keeps rules whose name, note, scope or conditions contain every query word. */
export function filterRules(rules: FwRule[], query: string): FwRule[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return rules
  return rules.filter((r) => {
    const text = ruleSearchText(r)
    return words.every((w) => text.includes(w))
  })
}

/** sortRules returns a sorted copy; 'default' keeps the server order. */
export function sortRules(rules: FwRule[], sort: RuleSort): FwRule[] {
  if (sort === 'default') return rules
  const out = [...rules]
  if (sort === 'name') {
    out.sort((a, b) => a.name.localeCompare(b.name))
  } else if (sort === 'hits') {
    out.sort((a, b) => b.hits.total - a.hits.total)
  } else {
    out.sort((a, b) =>
      (b.updated_at ?? b.created_at ?? '').localeCompare(
        a.updated_at ?? a.created_at ?? '',
      ),
    )
  }
  return out
}

export type SelectState = 'none' | 'some' | 'all'

export function selectState(
  visible: FwRule[],
  selected: Set<string>,
): SelectState {
  const n = visible.filter((r) => selected.has(r.id)).length
  if (n === 0) return 'none'
  return n === visible.length ? 'all' : 'some'
}
