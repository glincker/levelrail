import { useTranslation } from 'react-i18next'
import { SearchGlassIcon } from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import {
  CONNECTION_HEALTHS,
  CONNECTION_KINDS,
  CONNECTION_SCOPES,
  type ConnectionFilters,
} from '../../lib/connections'

export function FilterBar({
  filters,
  onChange,
}: Readonly<{
  filters: ConnectionFilters
  onChange: (next: ConnectionFilters) => void
}>) {
  const { t } = useTranslation('integrations')
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <div className="relative min-w-0 sm:max-w-xs sm:flex-1">
        <SearchGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          type="search"
          value={filters.query}
          onChange={(e) => {
            onChange({ ...filters, query: e.target.value })
          }}
          placeholder={t('filters.searchPlaceholder')}
          aria-label={t('filters.searchLabel')}
          className="pl-8"
        />
      </div>
      <div className="grid grid-cols-3 gap-2 sm:flex">
        <NativeSelect
          aria-label={t('filters.kind')}
          value={filters.kind}
          onChange={(e) => {
            onChange({
              ...filters,
              kind: e.target.value as ConnectionFilters['kind'],
            })
          }}
        >
          <option value="all">{t('filters.allKinds')}</option>
          {CONNECTION_KINDS.map((k) => (
            <option key={k} value={k}>
              {t(`kinds.${k}`)}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label={t('filters.scope')}
          value={filters.scope}
          onChange={(e) => {
            onChange({
              ...filters,
              scope: e.target.value as ConnectionFilters['scope'],
            })
          }}
        >
          <option value="all">{t('filters.allScopes')}</option>
          {CONNECTION_SCOPES.map((s) => (
            <option key={s} value={s}>
              {t(`scopes.${s}`)}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label={t('filters.health')}
          value={filters.health}
          onChange={(e) => {
            onChange({
              ...filters,
              health: e.target.value as ConnectionFilters['health'],
            })
          }}
        >
          <option value="all">{t('filters.allHealth')}</option>
          {CONNECTION_HEALTHS.map((h) => (
            <option key={h} value={h}>
              {t(`health.${h}`)}
            </option>
          ))}
        </NativeSelect>
      </div>
    </div>
  )
}
