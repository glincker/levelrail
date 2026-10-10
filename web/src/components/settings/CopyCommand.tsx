import { useState } from 'react'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'

export function CopyCommand({
  label,
  command,
}: {
  label: string
  command: string
}) {
  const { t } = useTranslation('updates')
  const [copied, setCopied] = useState(false)
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs whitespace-nowrap">
          {command}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            void navigator.clipboard.writeText(command).then(() => {
              setCopied(true)
              setTimeout(() => {
                setCopied(false)
              }, 2000)
            })
          }}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? t('copy.copied') : t('copy.copy')}
        </Button>
      </div>
    </div>
  )
}
