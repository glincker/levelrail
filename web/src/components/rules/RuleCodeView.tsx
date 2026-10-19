import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'

/** RuleCodeView shows the rule exactly as it is stored, read only, with copy as a secondary action. */
export function RuleCodeView({ code }: { code: string }) {
  const { t } = useTranslation('rules')
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">{t('builder.codeHelp')}</p>
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={() => void copy()}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? t('builder.copied') : t('builder.copy')}
        </Button>
      </div>
      <pre
        tabIndex={0}
        aria-label={t('builder.codeLabel')}
        className="max-h-72 overflow-auto rounded-lg bg-muted p-3 font-mono text-xs leading-5 text-foreground"
      >
        {code}
      </pre>
    </div>
  )
}
