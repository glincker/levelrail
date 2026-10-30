import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  KeyRotationGlassIcon,
  MessageSquareWarningGlassIcon,
  PowerGlassIcon,
  PowerOffGlassIcon,
  SendGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
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
import { FIELD_CLASS, PanelCard, Reason } from './mailPanelUi'

/** MailSystemEmailCard sends invites, resets and alerts as noreply@ a claimed domain through this mail server. */
export function MailSystemEmailCard({ domains }: { domains: MailDomain[] }) {
  const { t } = useTranslation('mailServer')
  const state = useSystemEmail().data
  const isRoot = useIsRoot()
  const invalidate = useInvalidateMail()
  const [domain, setDomain] = useState('')
  const [replace, setReplace] = useState(false)
  const [steps, setSteps] = useState<SystemEmailStep[]>([])
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('systemEmail.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)
  const chosen = domain || domains[0]?.domain || ''
  const run = (fn: () => Promise<{ steps: SystemEmailStep[] }>) => {
    setError(null)
    return gate.guard(systemEmailStepUp, async () => {
      const res = await fn()
      setSteps(res.steps)
      void invalidate()
    })
  }
  const rootWhy = isRoot ? undefined : t('systemEmail.needsRoot')
  const dns = state?.dns
  return (
    <section id={SYSTEM_EMAIL_ANCHOR}>
      <PanelCard label={t('systemEmail.title')} title={t('systemEmail.title')}>
        <p className="text-xs text-muted-foreground">
          {t('systemEmail.intro')}
        </p>
        {state?.enabled ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-medium">{state.from_address}</span>
            <span className="text-xs text-muted-foreground">
              {state.active
                ? t('systemEmail.active', { addr: state.submission })
                : t('systemEmail.replaced')}
            </span>
            <span className="ml-auto flex flex-wrap gap-1">
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
              <Reason why={rootWhy}>
                <Button
                  size="xs"
                  variant="outline"
                  disabled={!isRoot}
                  onClick={() => void run(() => systemEmailAction('disable'))}
                >
                  <PowerOffGlassIcon size="1.1em" />
                  {t('systemEmail.disable')}
                </Button>
              </Reason>
            </span>
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <select
              aria-label={t('systemEmail.domain')}
              className={FIELD_CLASS}
              value={chosen}
              onChange={(e) => setDomain(e.target.value)}
            >
              {domains.map((d) => (
                <option key={d.domain} value={d.domain}>
                  {t('systemEmail.fromOption', { domain: d.domain })}
                </option>
              ))}
            </select>
            <label className="flex items-center gap-1 text-xs">
              <input
                type="checkbox"
                checked={replace}
                onChange={(e) => setReplace(e.target.checked)}
              />
              {t('systemEmail.replace')}
            </label>
            <Reason
              why={rootWhy ?? (chosen ? undefined : t('systemEmail.noDomain'))}
            >
              <Button
                size="xs"
                disabled={!isRoot || !chosen}
                onClick={() =>
                  void run(() => enableSystemEmail(chosen, '', replace))
                }
              >
                <PowerGlassIcon size="1.1em" />
                {t('systemEmail.enable')}
              </Button>
            </Reason>
          </div>
        )}
        {state?.last_test ? (
          <p className="text-xs text-muted-foreground">
            {t(
              state.last_test.ok
                ? 'systemEmail.lastOk'
                : 'systemEmail.lastFail',
              {
                to: state.last_test.to,
                message: state.last_test.message,
              },
            )}
          </p>
        ) : null}
        {dns ? (
          <p className="text-xs text-muted-foreground">
            {t('systemEmail.dns', {
              spf: dns.spf,
              dkim: dns.dkim,
              dmarc: dns.dmarc,
            })}
          </p>
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
          <ul
            aria-label={t('systemEmail.steps')}
            className="space-y-0.5 text-xs"
          >
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
      </PanelCard>
    </section>
  )
}
