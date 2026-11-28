import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { ImportFinding, ImportFindingSeverity } from '../queries/imports'

const ORDER: ImportFindingSeverity[] = [
  'blocker',
  'changes-behavior',
  'ignored-safe',
]

const BADGE = {
  blocker: 'destructive',
  'changes-behavior': 'warning',
  'ignored-safe': 'muted',
} as const

/** Import fidelity report: what the import dropped or changed, worst first. */
export function ImportFindingsReport({
  findings,
}: {
  findings: ImportFinding[]
}) {
  const { t } = useTranslation('migration')
  if (findings.length === 0) return null
  const hasBlocker = findings.some((f) => f.severity === 'blocker')
  return (
    <Alert
      variant={hasBlocker ? 'destructive' : 'default'}
      data-testid="import-report"
    >
      <TriangleAlertGlassIcon size="1.7em" aria-hidden="true" />
      <AlertTitle>{t('importPlan.report.title')}</AlertTitle>
      <AlertDescription>
        <div className="space-y-3">
          {ORDER.map((sev) => {
            const rows = findings.filter((f) => f.severity === sev)
            if (rows.length === 0) return null
            const label = t(`importPlan.report.severity.${sev}`)
            return (
              <section key={sev} aria-label={label} className="space-y-1">
                <Badge variant={BADGE[sev]}>
                  {label} ({rows.length})
                </Badge>
                <ul className="space-y-1">
                  {rows.map((f, i) => (
                    <li
                      key={`${f.kind}-${f.path}-${String(i)}`}
                      className={
                        sev === 'blocker'
                          ? 'rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm'
                          : 'text-sm'
                      }
                    >
                      <span className="font-mono text-xs">{f.path}</span>
                      <span className="ml-2 text-xs text-muted-foreground">
                        {f.kind}
                      </span>
                      <span className="block">{f.reason}</span>
                      {f.fix ? (
                        <span className="block text-xs text-muted-foreground">
                          {t('importPlan.report.fix', { fix: f.fix })}
                        </span>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </section>
            )
          })}
        </div>
      </AlertDescription>
    </Alert>
  )
}
