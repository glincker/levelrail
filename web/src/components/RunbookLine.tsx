import { useTranslation } from 'react-i18next'

// Only http(s) becomes a link, so a stored note can never inject a javascript: URL.
function isWebUrl(value: string): boolean {
  try {
    const u = new URL(value)
    return u.protocol === 'https:' || u.protocol === 'http:'
  } catch {
    return false
  }
}

export function RunbookLine({ runbook }: { runbook?: string }) {
  const { t } = useTranslation('alertRunbook')
  if (!runbook) return null
  return (
    <p className="mt-0.5 text-xs text-muted-foreground">
      <span className="font-medium">{t('label')}: </span>
      {isWebUrl(runbook) ? (
        <a
          href={runbook}
          target="_blank"
          rel="noopener noreferrer"
          className="break-all text-primary underline underline-offset-2"
        >
          {runbook}
        </a>
      ) : (
        <span className="break-words">{runbook}</span>
      )}
    </p>
  )
}
