import { useTranslation } from 'react-i18next'
import { CodeBlock } from '@/components/ui/code-block'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { ProxySetupResult } from '../../queries/proxyIntegration'

export interface ProxyFailure {
  message: string
  status: number
  applying: boolean
}

const CONFLICT = 409

export function ProxyProviderResult({
  result,
  failure,
}: {
  result: ProxySetupResult | null
  failure: ProxyFailure | null
}) {
  const { t } = useTranslation('domains')
  if (failure) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
        <AlertTitle>
          {failure.applying
            ? t('proxyPicker.applyFailed')
            : t('proxyPicker.dryRunFailed')}
        </AlertTitle>
        <AlertDescription className="space-y-2">
          <pre className="font-mono text-xs break-words whitespace-pre-wrap">
            {failure.message}
          </pre>
          {failure.status === CONFLICT ? (
            <p>{t('proxyPicker.rejected.body')}</p>
          ) : null}
        </AlertDescription>
      </Alert>
    )
  }
  if (!result) return null
  if (result.manual) {
    return (
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-foreground">
          {t('proxyPicker.changes.manualTitle')}
        </h3>
        <CodeBlock
          code={result.snippet ?? ''}
          label={t('proxyPicker.changes.manualTitle')}
        />
        <p className="text-xs text-muted-foreground">
          {t('proxyPicker.manualNoApply')}
        </p>
      </div>
    )
  }
  const changes = result.changes ?? []
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-foreground">
        {t('proxyPicker.changes.title')}
      </h3>
      {changes.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('proxyPicker.changes.none')}
        </p>
      ) : (
        <ul className="list-disc space-y-1 pl-4 font-mono text-xs text-foreground">
          {changes.map((c) => (
            <li key={c} className="break-all">
              {c}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
