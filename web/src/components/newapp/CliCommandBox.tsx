import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

/** A monospace command with a copy button. */
export function CliCommandBox({ command }: { command: string }) {
  const { t } = useTranslation('newApp')
  const [copied, setCopied] = useState(false)

  function copy() {
    void navigator.clipboard
      .writeText(command)
      .then(() => {
        setCopied(true)
      })
      .catch(() => {
        setCopied(false)
      })
  }

  return (
    <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/40 p-2">
      <code className="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-nowrap text-foreground">
        {command}
      </code>
      <Button type="button" size="sm" variant="outline" onClick={copy}>
        {copied ? (
          <CheckGlassIcon className="size-5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-5" aria-hidden="true" />
        )}
        {copied ? t('upload.copied') : t('upload.copy')}
      </Button>
    </div>
  )
}
