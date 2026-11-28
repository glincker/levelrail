import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchGlassIcon } from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Switch } from '@/components/ui/switch'
import { SEARCH_DEBOUNCE_MS } from '../../lib/searchDebounce'
import { ABILITIES } from '../../types/token'
import type {
  StatusCounts,
  TokenKind,
  TokenSort,
  TokenStatus,
} from '../../types/tokenList'

export type StatusFilter = TokenStatus | 'all'
const STATUSES: StatusFilter[] = [
  'active',
  'expiring',
  'expired',
  'revoked',
  'all',
]
const KINDS: TokenKind[] = ['emergency', 'agent', 'system', 'user']
const SORTS: TokenSort[] = ['created', 'last_used', 'expires', 'name']

export interface ToolbarProps {
  q: string
  onQ: (q: string) => void
  status: StatusFilter
  onStatus: (s: StatusFilter) => void
  counts: StatusCounts | undefined
  total: number | undefined
  kind: TokenKind | ''
  onKind: (k: TokenKind | '') => void
  ability: string
  onAbility: (a: string) => void
  sort: TokenSort
  onSort: (s: TokenSort) => void
  grouped: boolean
  onGrouped: (on: boolean) => void
}

export function TokensToolbar(p: ToolbarProps) {
  const { t } = useTranslation('tokenList')
  const [draft, setDraft] = useState(p.q)
  const { q, onQ } = p

  // Debounced so each keystroke does not refetch a thousand-token list.
  useEffect(() => {
    if (draft === q) return undefined
    const id = window.setTimeout(() => {
      onQ(draft)
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(id)
    }
  }, [draft, q, onQ])

  const options = STATUSES.map((s) => {
    const n = s === 'all' ? p.total : p.counts?.[s]
    return {
      value: s,
      label: n === undefined ? t(`status.${s}`) : `${t(`status.${s}`)} ${n}`,
    }
  })

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-48 flex-1 basis-56">
        <SearchGlassIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          type="search"
          value={draft}
          aria-label={t('search')}
          placeholder={t('searchPlaceholder')}
          onChange={(e) => {
            setDraft(e.target.value)
          }}
          className="pl-8"
        />
      </div>
      <div className="max-w-full overflow-x-auto">
        <SegmentedControl
          aria-label={t('statusLabel')}
          value={p.status}
          onValueChange={p.onStatus}
          options={options}
          className="min-w-max"
        />
      </div>
      <NativeSelect
        aria-label={t('kindLabel')}
        value={p.kind}
        onChange={(e) => {
          p.onKind(e.target.value as TokenKind | '')
        }}
      >
        <option value="">{t('kind.all')}</option>
        {KINDS.map((k) => (
          <option key={k} value={k}>
            {t(`kind.${k}`)}
          </option>
        ))}
      </NativeSelect>
      <NativeSelect
        aria-label={t('abilityLabel')}
        value={p.ability}
        onChange={(e) => {
          p.onAbility(e.target.value)
        }}
      >
        <option value="">{t('abilityAll')}</option>
        {ABILITIES.map((a) => (
          <option key={a} value={a}>
            {a}
          </option>
        ))}
      </NativeSelect>
      <NativeSelect
        aria-label={t('sortLabel')}
        value={p.sort}
        disabled={p.grouped}
        onChange={(e) => {
          p.onSort(e.target.value as TokenSort)
        }}
      >
        {SORTS.map((s) => (
          <option key={s} value={s}>
            {t(`sort.${s}`)}
          </option>
        ))}
      </NativeSelect>
      <label className="flex items-center gap-2 text-sm">
        <Switch
          aria-label={t('groupBy')}
          checked={p.grouped}
          onCheckedChange={p.onGrouped}
        />
        {t('groupBy')}
      </label>
    </div>
  )
}
