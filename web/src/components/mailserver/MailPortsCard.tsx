import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { cn } from '@/lib/utils'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  mailPortsStepUpScope,
  useMailPorts,
  useOpenMailPorts,
  type MailPortRow,
} from '../../queries/mailPorts'
import { StepUpDialog } from '../env/StepUpDialog'

const BIND_TONE: Record<MailPortRow['bind'], string> = {
  public: 'text-tone-warning',
  private: 'text-muted-foreground',
  not_published: 'text-tone-danger',
}

export function MailPortsCard() {
  const { t } = useTranslation('mailServer')
  const ports = useMailPorts()
  const open = useOpenMailPorts()
  const [confirming, setConfirming] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const onGateError = useCallback(
    (e: unknown) =>
      setFailure(e instanceof Error ? e.message : t('ports.failed')),
    [t],
  )
  const gate = useStepUpGate(onGateError)
  const data = ports.data
  if (!data?.installed) return null
  const allPublic = data.ports.every((p) => p.bind === 'public')
  const missing = data.ports.some((p) => !p.published)

  const run = async () => {
    setFailure(null)
    setConfirming(false)
    await gate.guard(mailPortsStepUpScope(data.app), async () => {
      const res = await open.mutateAsync(data.app)
      setMessage(res.message)
    })
  }

  return (
    <section
      aria-label={t('ports.title')}
      className="rounded-lg border border-border bg-card p-3"
    >
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{t('ports.title')}</h2>
        <Button
          size="sm"
          variant={allPublic ? 'outline' : 'default'}
          disabled={allPublic || missing || open.isPending}
          onClick={() => setConfirming(true)}
        >
          {allPublic ? t('ports.opened') : t('ports.open')}
        </Button>
      </div>
      <ul
        aria-label={t('ports.listLabel')}
        className="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-2 xl:grid-cols-4"
      >
        {data.ports.map((p) => (
          <li key={p.port} className="flex items-baseline gap-2">
            <span className="font-mono font-medium">{p.port}</span>
            <span className={cn(BIND_TONE[p.bind])}>
              {t(`ports.bind.${p.bind}`)}
            </span>
            <span className="text-muted-foreground">
              {t(`ports.firewall.${p.firewall}`)}
            </span>
          </li>
        ))}
      </ul>
      <p className="mt-2 text-xs text-muted-foreground">
        {missing
          ? t('ports.missing')
          : t('ports.webStays', { bind: data.web_listener })}
      </p>
      {message && <p className="mt-1 text-xs">{message}</p>}
      {failure && (
        <p role="alert" className="mt-1 text-xs text-tone-danger">
          {failure}
        </p>
      )}
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('ports.confirmTitle')}
        description={data.warning}
        consequences={[
          t('ports.consequence'),
          data.port_25_note,
          t('ports.certificate'),
        ]}
        confirmLabel={t('ports.confirm')}
        pendingLabel={t('ports.pending')}
        pending={open.isPending}
        focusCancel
        onConfirm={() => void run()}
      />
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
