import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
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
          <CheckIcon className="size-4" aria-hidden="true" />
        ) : (
          <CopyIcon className="size-4" aria-hidden="true" />
        )}
        {copied ? t('upload.copied') : t('upload.copy')}
      </Button>
    </div>
  )
}
