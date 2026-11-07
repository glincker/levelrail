import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'

function snippetFor(app: string, key: string): string {
  const src = `${window.location.origin}/api/v1/vitals/snippet.js`
  return `<script defer src="${src}" data-app="${app}" data-key="${key}"></script>`
}

export function SnippetBlock({
  app,
  publicKey,
}: {
  app: string
  publicKey: string
}) {
  const { t } = useTranslation('webVitals')
  const { copied, copy } = useCopyToClipboard()
  const snippet = snippetFor(app, publicKey)
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{t('snippetHint')}</p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto rounded bg-muted px-2 py-1 font-mono text-xs whitespace-nowrap">
          {snippet}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-label={t('copySnippet')}
          onClick={() => copy(snippet)}
        >
          {copied ? (
            <CheckGlassIcon size="1em" />
          ) : (
            <CopyGlassIcon size="1em" />
          )}
          {copied ? t('copied') : t('copySnippet')}
        </Button>
      </div>
    </div>
  )
}
