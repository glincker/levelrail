import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ChevronRightGlassIcon,
  CopyGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

export interface CliStep {
  label?: string
  command: string
}

function CliLine({ step }: { step: CliStep }) {
  const { t } = useTranslation('fixit', { useSuspense: false })
  const [copied, setCopied] = useState(false)
  return (
    <div className="space-y-1">
      {step.label ? (
        <p className="text-xs text-muted-foreground">{step.label}</p>
      ) : null}
      <div className="flex items-start gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto rounded-md bg-muted px-2.5 py-2 font-mono text-xs whitespace-pre-wrap break-all">
          {step.command}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="shrink-0"
          onClick={() => {
            void navigator.clipboard.writeText(step.command).then(() => {
              setCopied(true)
            })
          }}
        >
          {copied ? (
            <CheckGlassIcon size="1.4em" />
          ) : (
            <CopyGlassIcon size="1.4em" />
          )}
          {copied ? t('copied') : t('copy')}
        </Button>
      </div>
    </div>
  )
}

/** FixCli is the secondary, collapsed "Fix via CLI" option; the commands are never the primary content. */
export function FixCli({ steps }: { steps: CliStep[] }) {
  const { t } = useTranslation('fixit', { useSuspense: false })
  if (steps.length === 0) return null
  return (
    <details className="group rounded-md border border-border/60">
      <summary className="flex cursor-pointer list-none items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium text-muted-foreground select-none">
        <ChevronRightGlassIcon
          className="size-4.5 transition-transform group-open:rotate-90"
          aria-hidden="true"
        />
        {t('cli.summary')}
      </summary>
      <div className="space-y-2 border-t border-border/60 p-2.5">
        <p className="text-xs text-muted-foreground">{t('cli.hint')}</p>
        {steps.map((step) => (
          <CliLine key={step.command} step={step} />
        ))}
      </div>
    </details>
  )
}
