import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'

/** Prefilled _redirects line for a missing path; the rules are edited in the published files, so this is a copy shortcut. */
export function RedirectHint({ from }: Readonly<{ from: string }>) {
  const { t } = useTranslation('pagesSites')
  const { copied, copy } = useCopyToClipboard()
  const line = `${from} / 301`
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-sm">
      <span className="text-muted-foreground">{t('redirect.hint')}</span>
      <code className="font-mono text-xs">{line}</code>
      <Button
        type="button"
        size="sm"
        variant="outline"
        aria-label={t('redirect.copy')}
        onClick={() => copy(line)}
      >
        {copied ? (
          <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-4" aria-hidden="true" />
        )}
        {t('redirect.copy')}
      </Button>
    </div>
  )
}
