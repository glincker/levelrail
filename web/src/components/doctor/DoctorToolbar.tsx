import { useTranslation } from 'react-i18next'
import { CopyGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import type { SeverityCounts } from '../../lib/doctorModel'
import type { DoctorFilter } from '../../lib/doctorDiagnostics'

const CHIPS: DoctorFilter[] = ['all', 'fail', 'warn']

/** DoctorToolbar is the summary strip: counts as filter chips, last checked time and Copy diagnostics. */
export function DoctorToolbar({
  counts,
  total,
  filter,
  onFilter,
  checkedAt,
  running,
  diagnostics,
}: {
  counts: SeverityCounts
  total: number
  filter: DoctorFilter
  onFilter: (f: DoctorFilter) => void
  checkedAt: Date | null
  running: boolean
  diagnostics: () => string
}) {
  const { t } = useTranslation('settings')

  function copy() {
    navigator.clipboard.writeText(diagnostics()).then(
      () => toast.add({ title: t('doctorPage.copyDone'), type: 'success' }),
      () => toast.add({ title: t('doctorPage.copyFailed'), type: 'error' }),
    )
  }

  const countFor: Record<DoctorFilter, number> = {
    all: total,
    fail: counts.fail,
    warn: counts.warn,
  }

  return (
    <div
      className="flex flex-wrap items-center gap-2"
      role="group"
      aria-label={t('doctorPage.filterLabel')}
    >
      {CHIPS.map((id) => (
        <Button
          key={id}
          size="sm"
          variant={filter === id ? 'default' : 'outline'}
          aria-pressed={filter === id}
          onClick={() => onFilter(id)}
        >
          {t(`doctorPage.filter.${id}`)}
          <Badge
            variant={
              id === 'fail'
                ? 'destructive'
                : id === 'warn'
                  ? 'warning'
                  : 'muted'
            }
          >
            {countFor[id]}
          </Badge>
        </Button>
      ))}
      {counts.unknown > 0 ? (
        <Badge variant="muted">
          {t('doctorPage.counts.unknown', { count: counts.unknown })}
        </Badge>
      ) : null}
      <span className="ml-auto text-xs text-muted-foreground">
        {running
          ? t('doctorPage.checking')
          : checkedAt
            ? t('doctorPage.lastChecked', {
                time: checkedAt.toLocaleTimeString(),
              })
            : null}
      </span>
      <Button variant="outline" size="sm" onClick={copy} disabled={running}>
        <CopyGlassIcon size="1.2em" />
        {t('doctorPage.copyDiagnostics')}
      </Button>
    </div>
  )
}
