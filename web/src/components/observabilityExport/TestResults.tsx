import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CircleAlertGlassIcon } from '@thesvg/ui-react'
import type { ExportTestResult } from '../../queries/observabilityExports'

export function TestResults({
  result,
}: Readonly<{ result: ExportTestResult }>) {
  const { t } = useTranslation('settings')
  return (
    <ul className="space-y-1.5" aria-label={t('obsExport.test.title')}>
      {result.results.map((r) => (
        <li
          key={r.signal}
          className="flex items-start gap-2 rounded-md border px-2.5 py-1.5 text-sm"
        >
          {r.ok ? (
            <CheckGlassIcon
              className="mt-0.5 size-4.5 shrink-0 text-tone-success"
              aria-hidden="true"
            />
          ) : (
            <CircleAlertGlassIcon
              className="mt-0.5 size-4.5 shrink-0 text-tone-danger"
              aria-hidden="true"
            />
          )}
          <div className="min-w-0">
            <span className="font-medium">
              {t(`obsExport.signal.${r.signal}`)}
            </span>
            <span className="text-muted-foreground">
              {' '}
              {r.ok
                ? t('obsExport.test.pass')
                : (r.hint ?? r.error ?? t('obsExport.test.failed'))}
            </span>
            {!r.ok && r.fix ? (
              <p className="text-xs font-medium text-tone-danger">
                {t(`obsExport.fix.${r.fix}`)}
              </p>
            ) : null}
          </div>
        </li>
      ))}
    </ul>
  )
}
