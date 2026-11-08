import { useMemo, useState } from 'react'
import type { FwRule } from '../../queries/firewallEdge'
import {
  filterRules,
  selectState,
  sortRules,
  type RuleSort,
  type SelectState,
} from './firewallListModel'

export interface RuleList {
  visible: FwRule[]
  query: string
  setQuery: (q: string) => void
  sort: RuleSort
  setSort: (s: RuleSort) => void
  /** True while search or sort changes the order, so manual reordering is off. */
  reordered: boolean
  selected: FwRule[]
  isSelected: (id: string) => boolean
  toggle: (id: string, on: boolean) => void
  state: SelectState
  setAll: (on: boolean) => void
  clear: () => void
}

/** useRuleList owns search, sort and selection for one rule list; selection only counts rules still present. */
export function useRuleList(rules: FwRule[]): RuleList {
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<RuleSort>('default')
  const [ids, setIds] = useState<Set<string>>(new Set())

  const visible = useMemo(
    () => sortRules(filterRules(rules, query), sort),
    [rules, query, sort],
  )
  const live = useMemo(() => new Set(rules.map((r) => r.id)), [rules])
  const effective = useMemo(
    () => new Set([...ids].filter((id) => live.has(id))),
    [ids, live],
  )

  return {
    visible,
    query,
    setQuery,
    sort,
    setSort,
    reordered: query.trim() !== '' || sort !== 'default',
    selected: rules.filter((r) => effective.has(r.id)),
    isSelected: (id) => effective.has(id),
    toggle: (id, on) =>
      setIds((prev) => {
        const next = new Set(prev)
        if (on) next.add(id)
        else next.delete(id)
        return next
      }),
    state: selectState(visible, effective),
    setAll: (on) => setIds(on ? new Set(visible.map((r) => r.id)) : new Set()),
    clear: () => setIds(new Set()),
  }
}
