import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  useCoolifyExitReport,
  type CoolifyExitReport,
  type ExitOutcome,
  type ExitRow,
} from '../queries/coolifyExit'

const outcomeVariant: Record<
  ExitOutcome,
  'success' | 'warning' | 'destructive'
> = {
  imported: 'success',
  manual: 'warning',
  skipped: 'destructive',
}

const outcomeOrder: ExitOutcome[] = ['skipped', 'manual', 'imported']

function Row({ row }: { row: ExitRow }) {
  const { t } = useTranslation('migration')
  return (
    <li className="space-y-1 rounded-md border p-2 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={outcomeVariant[row.outcome]}>
          {t(`appImport.exit.outcome.${row.outcome}`)}
        </Badge>
        <span className="text-xs text-muted-foreground">{row.kind}</span>
        <span className="font-medium">{row.name}</span>
        {row.target && row.target !== row.name ? (
          <code className="text-xs">{row.target}</code>
        ) : null}
      </div>
      {row.reasons?.map((r) => (
        <p key={r} className="text-xs text-muted-foreground">
          {r}
        </p>
      ))}
      {row.next?.map((n) => (
        <p key={n} className="text-xs">
          {t('appImport.exit.next')} {n}
        </p>
      ))}
    </li>
  )
}

function Body({ report }: { report: CoolifyExitReport }) {
  const { t } = useTranslation('migration')
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        {outcomeOrder.map((o) => (
          <Badge key={o} variant={outcomeVariant[o]}>
            {t(`appImport.exit.outcome.${o}`)} {report.counts[o] ?? 0}
          </Badge>
        ))}
        <Badge variant="muted">{t(`appImport.exit.mode.${report.mode}`)}</Badge>
      </div>
      {report.ready_to_retire ? (
        <Alert>
          <AlertDescription>{t('appImport.exit.ready')}</AlertDescription>
        </Alert>
      ) : null}
      {outcomeOrder.map((o) => {
        const rows = report.rows.filter((r) => r.outcome === o)
        return rows.length > 0 ? (
          <ul key={o} className="space-y-1.5">
            {rows.map((r) => (
              <Row key={`${r.kind}-${r.name}`} row={r} />
            ))}
          </ul>
        ) : null
      })}
      {report.waves.length > 0 ? (
        <div className="space-y-2">
          <h4 className="text-sm font-semibold">
            {t('appImport.exit.wavesTitle')}
          </h4>
          <ol className="space-y-2">
            {report.waves.map((w) => (
              <li key={w.number} className="text-sm">
                <p className="font-medium">
                  {w.number}. {w.title}
                </p>
                <p className="text-xs text-muted-foreground">{w.why}</p>
                <p className="text-xs">
                  {w.apps
                    .map((a) => [a.name, ...(a.domains ?? [])].join(' '))
                    .join(', ')}
                </p>
              </li>
            ))}
          </ol>
        </div>
      ) : null}
      <div className="space-y-2">
        <h4 className="text-sm font-semibold">
          {t('appImport.exit.checklistTitle')}
        </h4>
        <ol className="list-decimal space-y-2 pl-5">
          {report.checklist.map((s) => (
            <li key={s.id} className="text-sm">
              <p className="font-medium">{s.title}</p>
              <p className="text-xs text-muted-foreground">{s.detail}</p>
            </li>
          ))}
        </ol>
      </div>
    </div>
  )
}

export function CoolifyExitReportPanel({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation('migration')
  const [open, setOpen] = useState(false)
  const q = useCoolifyExitReport(sessionId, open)
  return (
    <div className="space-y-3 rounded-md border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="space-y-0.5">
          <h3 className="text-sm font-semibold">{t('appImport.exit.title')}</h3>
          <p className="text-xs text-muted-foreground">
            {t('appImport.exit.help')}
          </p>
        </div>
        <Button
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={q.isFetching}
          onClick={() => (open ? void q.refetch() : setOpen(true))}
        >
          {open ? t('appImport.exit.recheck') : t('appImport.exit.show')}
        </Button>
      </div>
      {q.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{q.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {q.data ? <Body report={q.data} /> : null}
    </div>
  )
}
