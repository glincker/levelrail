import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  KeyRotationGlassIcon,
  MessageSquareWarningGlassIcon,
  SendGlassIcon,
  SlidersHorizontalGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import { useInvalidateMail, type MailDomain } from '../../queries/mailMailboxes'
import {
  enableSystemEmail,
  SYSTEM_EMAIL_ANCHOR,
  systemEmailAction,
  systemEmailStepUp,
  useSystemEmail,
  type SystemEmailStep,
} from '../../queries/mailSystemEmail'
import { StepUpDialog } from '../env/StepUpDialog'
import { FIELD_CLASS, Reason } from './mailPanelUi'

/** One row: a switch to send platform emails from noreply@domain, with options behind Settings. */
export function MailSystemEmailCard({ domains }: { domains: MailDomain[] }) {
  const { t } = useTranslation('mailServer')
  const isRoot = useIsRoot()
  const state = useSystemEmail(isRoot).data
  const invalidate = useInvalidateMail()
  const [domain, setDomain] = useState('')
  const [replace, setReplace] = useState(false)
  const [showOptions, setShowOptions] = useState(false)
  const [steps, setSteps] = useState<SystemEmailStep[]>([])
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('systemEmail.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)
  const chosen = domain || domains[0]?.domain || ''
  const enabled = !!state?.enabled
  const from = state?.from_address ?? (chosen ? `noreply@${chosen}` : '')
  const run = (fn: () => Promise<{ steps: SystemEmailStep[] }>) => {
    setError(null)
    return gate.guard(systemEmailStepUp, async () => {
      const res = await fn()
      setSteps(res.steps)
      void invalidate()
    })
  }
  const rootWhy = isRoot ? undefined : t('systemEmail.needsRoot')
  const onWhy =
    rootWhy ?? (chosen || enabled ? undefined : t('systemEmail.noDomain'))
  const dns = state?.dns
  const last = state?.last_test
  const statusKey = !enabled
    ? 'systemEmail.statusOff'
    : state?.active
      ? 'systemEmail.statusOn'
      : 'systemEmail.statusReplaced'
  const tone = !enabled
    ? 'bg-muted-foreground/40'
    : state?.active
      ? 'bg-tone-success'
      : 'bg-tone-warning'

  return (
    <section
      id={SYSTEM_EMAIL_ANCHOR}
      aria-label={t('systemEmail.title')}
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center gap-3">
        <Reason why={onWhy}>
          <Switch
            size="sm"
            aria-label={t('systemEmail.switchLabel', { from })}
            checked={enabled}
            disabled={!isRoot || (!enabled && !chosen)}
            onCheckedChange={(on) =>
              void run(() =>
                on
                  ? enableSystemEmail(chosen, '', replace)
                  : systemEmailAction('disable'),
              )
            }
          />
        </Reason>
        <span className="text-sm font-medium">
          {t('systemEmail.switchLabel', { from })}
        </span>
        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span
            aria-hidden="true"
            className={cn('size-2 rounded-full', tone)}
          />
          {t(statusKey)}
          {' · '}
          {last
            ? t(last.ok ? 'systemEmail.lastOk' : 'systemEmail.lastFail', {
                to: last.to,
                message: last.message,
              })
            : t('systemEmail.noLast')}
        </span>
        <span className="ml-auto flex items-center gap-1">
          {enabled ? (
            <Reason why={rootWhy}>
              <Button
                size="xs"
                variant="outline"
                disabled={!isRoot}
                onClick={() => void run(() => systemEmailAction('test'))}
              >
                <SendGlassIcon size="1.1em" />
                {t('systemEmail.test')}
              </Button>
            </Reason>
          ) : null}
          <Button
            size="xs"
            variant="ghost"
            aria-expanded={showOptions}
            onClick={() => setShowOptions((v) => !v)}
          >
            <SlidersHorizontalGlassIcon size="1.1em" />
            {t('systemEmail.settings')}
          </Button>
        </span>
      </div>
      {showOptions ? (
        <div className="space-y-3 border-t border-border pt-3 text-xs">
          {!enabled ? (
            <>
              <div className="flex items-center gap-2">
                <label htmlFor="sys-domain" className="text-muted-foreground">
                  {t('systemEmail.domain')}
                </label>
                <select
                  id="sys-domain"
                  className={`${FIELD_CLASS} min-w-56`}
                  value={chosen}
                  onChange={(e) => setDomain(e.target.value)}
                >
                  {domains.map((d) => (
                    <option key={d.domain} value={d.domain}>
                      {t('systemEmail.fromOption', { domain: d.domain })}
                    </option>
                  ))}
                </select>
              </div>
              <label className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-0.5"
                  checked={replace}
                  onChange={(e) => setReplace(e.target.checked)}
                />
                <span>
                  <span className="block font-medium">
                    {t('systemEmail.replace')}
                  </span>
                  <span className="block text-muted-foreground">
                    {t('systemEmail.replaceHelp')}
                  </span>
                </span>
              </label>
            </>
          ) : (
            <div className="space-y-1">
              <p className="text-muted-foreground">
                {t('systemEmail.pinNote')}
              </p>
              <Reason why={rootWhy}>
                <Button
                  size="xs"
                  variant="outline"
                  disabled={!isRoot}
                  onClick={() => void run(() => systemEmailAction('rotate'))}
                >
                  <KeyRotationGlassIcon size="1.1em" />
                  {t('systemEmail.rotate')}
                </Button>
              </Reason>
            </div>
          )}
          {dns ? (
            <ul
              aria-label={t('systemEmail.dnsReady')}
              className="flex flex-wrap gap-x-4 gap-y-1"
            >
              {(['spf', 'dkim', 'dmarc'] as const).map((k) => (
                <li key={k}>
                  {t('systemEmail.dnsRow', {
                    name: k.toUpperCase(),
                    value: dns[k],
                  })}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
      {state?.warning ? (
        <p
          role="note"
          className="flex items-start gap-1 text-xs text-amber-600 dark:text-amber-400"
        >
          <MessageSquareWarningGlassIcon size="1.1em" />
          <span>
            {t('systemEmail.dnsWarning')}{' '}
            <a href="#mail-dns-panel" className="underline">
              {t('systemEmail.dnsLink')}
            </a>
          </span>
        </p>
      ) : null}
      {steps.length > 0 ? (
        <ul aria-label={t('systemEmail.steps')} className="space-y-0.5 text-xs">
          {steps.map((s) => (
            <li key={s.id} className={s.ok ? '' : 'text-destructive'}>
              {t(`systemEmail.step.${s.id}`, { defaultValue: s.id })}:{' '}
              {s.message}
            </li>
          ))}
        </ul>
      ) : null}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </section>
  )
}
