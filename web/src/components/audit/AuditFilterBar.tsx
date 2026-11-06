import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AUDIT_RANGES, isAuditRange, type AuditRange } from '@/queries/auditLog'

const ANY_VALUE = '__any__'
const METHODS = ['POST', 'PUT', 'PATCH', 'DELETE'] as const

export interface AuditFilterValue {
  range?: AuditRange
  actor: string
  method?: string
}

interface Props {
  value: AuditFilterValue
  onChange: (next: AuditFilterValue) => void
}

// Time range, method and actor; search, status and client stay on the page row.
export function AuditFilterBar({ value, onChange }: Props) {
  const { t } = useTranslation('auditLog')
  return (
    <>
      <Select
        value={value.range ?? ANY_VALUE}
        onValueChange={(next) => {
          onChange({ ...value, range: isAuditRange(next) ? next : undefined })
        }}
      >
        <SelectTrigger aria-label={t('filters.rangeLabel')} className="w-36">
          <SelectValue placeholder={t('filters.anyTime')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ANY_VALUE}>{t('filters.anyTime')}</SelectItem>
          {AUDIT_RANGES.map((r) => (
            <SelectItem key={r} value={r}>
              {t(`filters.range.${r}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={value.method ?? ANY_VALUE}
        onValueChange={(next) => {
          onChange({
            ...value,
            method: next && next !== ANY_VALUE ? next : undefined,
          })
        }}
      >
        <SelectTrigger aria-label={t('filters.methodLabel')} className="w-32">
          <SelectValue placeholder={t('filters.anyMethod')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ANY_VALUE}>{t('filters.anyMethod')}</SelectItem>
          {METHODS.map((m) => (
            <SelectItem key={m} value={m}>
              {m}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        value={value.actor}
        onChange={(e) => {
          onChange({ ...value, actor: e.target.value })
        }}
        placeholder={t('filters.actorPlaceholder')}
        aria-label={t('filters.actorLabel')}
        className="w-40"
      />
    </>
  )
}
