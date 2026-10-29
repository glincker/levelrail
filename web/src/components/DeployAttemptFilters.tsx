import { useTranslation } from 'react-i18next'
import { SearchGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  EMPTY_DEPLOY_FILTERS,
  distinctDeployValues,
  hasActiveDeployFilters,
  type DeployAttemptFilters,
} from '../lib/deployAttemptFilter'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const ALL = 'all'

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: string[]
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('common')
  if (options.length === 0) return null
  return (
    <Select
      value={value || ALL}
      onValueChange={(v) => onChange(!v || v === ALL ? '' : v)}
    >
      <SelectTrigger className="w-full sm:w-40" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>
          {t('deployHistory.filters.all', { label })}
        </SelectItem>
        {options.map((o) => (
          <SelectItem key={o} value={o}>
            {o}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function DeployAttemptFilterBar({
  attempts,
  filters,
  onChange,
}: {
  attempts: DeployAttempt[]
  filters: DeployAttemptFilters
  onChange: (next: DeployAttemptFilters) => void
}) {
  const { t } = useTranslation('common')
  return (
    <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
      <div className="relative w-full sm:w-64">
        <SearchGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          className="pl-8"
          value={filters.query}
          placeholder={t('deployHistory.filters.search')}
          aria-label={t('deployHistory.filters.search')}
          onChange={(e) => onChange({ ...filters, query: e.target.value })}
        />
      </div>
      <FilterSelect
        label={t('deployHistory.filters.branch')}
        value={filters.branch}
        options={distinctDeployValues(attempts, 'branch')}
        onChange={(branch) => onChange({ ...filters, branch })}
      />
      <FilterSelect
        label={t('deployHistory.filters.author')}
        value={filters.author}
        options={distinctDeployValues(attempts, 'author')}
        onChange={(author) => onChange({ ...filters, author })}
      />
      <FilterSelect
        label={t('deployHistory.filters.status')}
        value={filters.status}
        options={distinctDeployValues(attempts, 'status')}
        onChange={(status) => onChange({ ...filters, status })}
      />
      {hasActiveDeployFilters(filters) ? (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onChange(EMPTY_DEPLOY_FILTERS)}
        >
          <XGlassIcon className="size-5" data-icon="inline-start" />
          {t('deployHistory.filters.clear')}
        </Button>
      ) : null}
    </div>
  )
}
