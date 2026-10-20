import { useTranslation } from 'react-i18next'

// The fix hint of a failed data copy, with the raw error one click away.
// The raw text is already redacted by the server and never the default view.
export function CopyFailureDetail({
  hint,
  technical,
}: {
  hint?: string
  technical?: string
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  if (!hint && !technical) return null
  return (
    <div className="space-y-1">
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      {technical ? (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer select-none">
            {t('copyFailure.technical')}
          </summary>
          <p className="mt-1">{t('copyFailure.technicalHelp')}</p>
          <pre className="mt-1 max-h-40 overflow-auto rounded-md bg-muted p-2 font-mono whitespace-pre-wrap break-all">
            {technical}
          </pre>
        </details>
      ) : null}
    </div>
  )
}
