import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { COPY_FEEDBACK_MS } from '../../lib/copyFeedback'
import {
  useNextAdvisor,
  type AdvisorFinding,
  type AdvisorSeverity,
} from '../../queries/nextReport'

const SEVERITY_VARIANT: Record<
  AdvisorSeverity,
  'info' | 'warning' | 'destructive'
> = { info: 'info', warn: 'warning', error: 'destructive' }

// A fix is shown as a command when it starts with the CLI name or carries flags.
function isCommand(fix: string): boolean {
  return /^levelrail\s/.test(fix) || /\s--[a-z]/.test(fix)
}

function CopyableCommand({ command }: { command: string }) {
  const { t } = useTranslation('resources', { keyPrefix: 'nextStatus' })
  const [copied, setCopied] = useState(false)
  const copy = () => {
    void navigator.clipboard
      .writeText(command)
      .then(() => {
        setCopied(true)
        window.setTimeout(() => {
          setCopied(false)
        }, COPY_FEEDBACK_MS)
      })
      .catch(() => {
        toast.add({ title: t('advisor.copyFailed'), type: 'error' })
      })
  }
  return (
    <span className="flex min-w-0 items-center gap-1">
      <code className="min-w-0 truncate rounded-md bg-muted px-1.5 py-0.5 font-mono text-xs">
        {command}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={copy}
        aria-label={t('advisor.copy')}
      >
        {copied ? (
          <CheckGlassIcon size="1.2em" />
        ) : (
          <CopyGlassIcon size="1.2em" />
        )}
      </Button>
    </span>
  )
}

function Finding({ f }: { f: AdvisorFinding }) {
  const { t } = useTranslation('resources', { keyPrefix: 'nextStatus' })
  return (
    <li className="space-y-1 px-3 py-2 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={SEVERITY_VARIANT[f.severity]}>
          {t(`advisor.severity.${f.severity}`)}
        </Badge>
        <span className="font-medium">{f.title}</span>
      </div>
      <p className="text-muted-foreground">{f.detail}</p>
      {f.fix ? (
        isCommand(f.fix) ? (
          <CopyableCommand command={f.fix} />
        ) : (
          <p>{f.fix}</p>
        )
      ) : null}
    </li>
  )
}

export function NextAdvisor({ appName }: { appName: string }) {
  const { t } = useTranslation('resources', { keyPrefix: 'nextStatus' })
  const q = useNextAdvisor(appName)
  if (!q.data || !q.data.detected) return null
  const findings = q.data.findings
  return (
    <li className="px-3 py-2 text-sm">
      <p className="mb-1 font-medium">{t('advisor.title')}</p>
      {findings.length === 0 ? (
        <p className="text-muted-foreground">{t('advisor.empty')}</p>
      ) : (
        <ul className="divide-y divide-border rounded-md border border-border">
          {findings.map((f) => (
            <Finding key={f.id} f={f} />
          ))}
        </ul>
      )}
    </li>
  )
}
