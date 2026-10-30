import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { COPY_FEEDBACK_MS } from '@/lib/copyFeedback'
import type { MailCheckReport } from '@/queries/mailDns'
import type { MailStatus } from '@/queries/mailStatus'
import { nextStepOf } from './mailSetup'

/** MailNextStep shows the single next action from the live status, with a prefilled CLI line. */
export function MailNextStep({
  status,
  report,
  domain,
  onPreview,
}: {
  status: MailStatus | undefined
  report: MailCheckReport | undefined
  domain: string
  onPreview?: () => void
}) {
  const { t } = useTranslation('mailServer')
  const [copied, setCopied] = useState(false)
  if (!status) return null
  const step = nextStepOf(status, report)
  const cmd = `lrcli templates install mail-mox --env MAIL_FIRST_DOMAIN=${domain || '<domain>'}`
  const done: ('installed' | 'running' | 'admin')[] = []
  if (status.installed) done.push('installed')
  if (status.container_state === 'running') done.push('running')
  if (status.admin_reachable) done.push('admin')

  const copy = () => {
    void navigator.clipboard?.writeText(cmd).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), COPY_FEEDBACK_MS)
    })
  }

  return (
    <section
      aria-label={t('next.title')}
      className="rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 className="text-sm font-medium">
          {step ? t(`next.step.${step}`) : t('next.allSet')}
        </h2>
      </div>
      {step && step !== 'dns' && step !== 'verify' && (
        <p className="mt-1 text-xs text-muted-foreground">{status.next_step}</p>
      )}
      {status.installed && status.tls === 'self-signed' && (
        <p className="mt-1 text-xs text-tone-warning">
          {t('next.tlsSelfSigned')}
        </p>
      )}
      {step === 'install' && (
        <div className="mt-2 flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded border border-border bg-muted px-2 py-1 font-mono text-xs">
            {cmd}
          </code>
          <button
            type="button"
            onClick={copy}
            className="inline-flex shrink-0 items-center gap-1.5 rounded border border-border px-2 py-1 text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
          >
            <CopyGlassIcon size="1.2em" />
            {copied ? t('next.copied') : t('next.copy')}
          </button>
        </div>
      )}
      {step === 'dns' && onPreview && (
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
          <Button type="button" size="sm" onClick={onPreview}>
            {t('next.cta')}
          </Button>
          <p className="text-xs text-muted-foreground">{t('next.ctaBody')}</p>
        </div>
      )}
      {done.length > 0 && (
        <ul
          aria-label={t('next.doneLabel')}
          className="mt-2 flex flex-wrap gap-x-4 gap-y-0.5 text-xs"
        >
          {done.map((k) => (
            <li key={k} className="flex items-center gap-1.5">
              <CircleCheckGlassIcon
                className="text-tone-success"
                size="1.2em"
              />
              {t(`next.doneItem.${k}`)}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
