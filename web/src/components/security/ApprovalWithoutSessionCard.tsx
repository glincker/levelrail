import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import { useBrand } from '../../hooks/useBrand'
import {
  type ApprovalWithoutSession,
  type SecurityPolicy,
  approvalPolicyStepUp,
  useUpdateSecurityPolicy,
} from '../../queries/securityCenter'
import { StepUpDialog } from '../env/StepUpDialog'

interface Pending {
  mode: ApprovalWithoutSession
  confirmHost: boolean
}

// Hold changes who can sign in, so it asks for a fresh password check and a way back in.
export function ApprovalWithoutSessionCard({
  policy,
  editable,
}: {
  policy: SecurityPolicy
  editable: boolean
}) {
  const { t } = useTranslation('security')
  const brand = useBrand()
  const update = useUpdateSecurityPolicy()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmHost, setConfirmHost] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [stepUp, setStepUp] = useState<Pending | null>(null)
  const current = policy.approval_without_session
  const channels = policy.approval_channels
  const resetCommand = `${brand.BinaryName || 'levelrail'} security reset-approval-policy`

  const apply = (p: Pending) => {
    setError(null)
    update.mutate(
      {
        approval_without_session: p.mode,
        ...(p.mode === 'hold' ? { confirm_host_access: p.confirmHost } : {}),
      },
      {
        onSuccess: () => {
          setConfirmOpen(false)
          setConfirmHost(false)
          toast.add({ title: t('withoutSession.saved'), type: 'success' })
        },
        onError: (err) => {
          if (err instanceof ApiError && err.code === 'step_up_required') {
            setStepUp(p)
            return
          }
          if (
            err instanceof ApiError &&
            err.code === 'approval_hold_no_channel'
          ) {
            setError(t('withoutSession.noChannelFix'))
            return
          }
          const msg = err.message || t('withoutSession.saveError')
          if (confirmOpen) setError(msg)
          else toast.add({ title: msg, type: 'error' })
        },
      },
    )
  }

  const choose = (mode: ApprovalWithoutSession) => {
    if (mode === current) return
    if (mode === 'hold') {
      setError(null)
      setConfirmOpen(true)
      return
    }
    apply({ mode, confirmHost: false })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('withoutSession.title')}</CardTitle>
        <CardDescription>{t('withoutSession.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <SegmentedControl<ApprovalWithoutSession>
          aria-label={t('withoutSession.title')}
          value={current}
          disabled={!editable || update.isPending}
          onValueChange={choose}
          options={[
            { value: 'allow', label: t('withoutSession.allow') },
            { value: 'hold', label: t('withoutSession.hold') },
          ]}
        />
        <p className="text-muted-foreground">
          {current === 'hold'
            ? t('withoutSession.holdBody')
            : t('withoutSession.allowBody')}
        </p>
        <p className="text-muted-foreground">
          {t('withoutSession.safeguards')}
        </p>
        <p className="text-muted-foreground">
          {t('withoutSession.reset')}{' '}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
            {resetCommand}
          </code>
        </p>
        {editable ? null : (
          <p className="text-muted-foreground">{t('policy.adminOnly')}</p>
        )}
      </CardContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          setConfirmOpen(open)
          if (!open) {
            setError(null)
            setConfirmHost(false)
          }
        }}
        title={t('withoutSession.confirmTitle')}
        description={t('withoutSession.confirmBody')}
        confirmLabel={t('withoutSession.confirm')}
        pendingLabel={t('withoutSession.confirming')}
        pending={update.isPending}
        error={error}
        focusCancel
        onConfirm={() => apply({ mode: 'hold', confirmHost })}
      >
        <div className="space-y-2 text-sm">
          <p className="font-medium">{t('withoutSession.pathsTitle')}</p>
          <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
            {channels.map((c) => (
              <li key={c}>{t(`withoutSession.channel.${c}`)}</li>
            ))}
          </ul>
          <label className="flex items-start gap-2">
            <Checkbox
              checked={confirmHost}
              onCheckedChange={(v) => setConfirmHost(v === true)}
              aria-label={t('withoutSession.hostAccess')}
            />
            <span>{t('withoutSession.hostAccess')}</span>
          </label>
        </div>
      </ConfirmDialog>
      <StepUpDialog
        scope={stepUp ? approvalPolicyStepUp : null}
        description={t('withoutSession.stepUp')}
        onVerified={() => {
          const p = stepUp
          setStepUp(null)
          if (p) apply(p)
        }}
        onCancel={() => setStepUp(null)}
      />
    </Card>
  )
}
