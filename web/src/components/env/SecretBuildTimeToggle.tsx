import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useSetSecretBuildTime } from '../../queries/secrets'

interface Props {
  appName: string
  secretKey: string
  enabled: boolean
  canEdit: boolean
}

// Per-secret opt-in for build.secrets. Only turning it on asks for
// confirmation; read-only viewers just see a badge on enabled rows.
export function SecretBuildTimeToggle({
  appName,
  secretKey,
  enabled,
  canEdit,
}: Props) {
  const { t } = useTranslation('secretRotation')
  const [confirming, setConfirming] = useState(false)
  const mutation = useSetSecretBuildTime(appName)

  const apply = (next: boolean, onDone?: () => void) => {
    mutation.mutate(
      { key: secretKey, enabled: next },
      {
        onSuccess: onDone,
        onError: (e) => {
          onDone?.()
          toast.add({
            title: t('buildTime.toast.failed', {
              key: secretKey,
              message: e.message,
            }),
            type: 'error',
          })
        },
      },
    )
  }

  if (!canEdit) {
    return enabled ? (
      <Badge variant="muted" title={t('buildTime.hint')}>
        {t('buildTime.label')}
      </Badge>
    ) : null
  }

  return (
    <>
      <label
        className="flex items-center gap-1.5 text-xs text-muted-foreground"
        title={t('buildTime.hint')}
      >
        {t('buildTime.label')}
        <Switch
          size="sm"
          checked={enabled}
          aria-label={`${t('buildTime.label')} ${secretKey}`}
          aria-description={t('buildTime.hint')}
          onCheckedChange={(next) => {
            if (next) setConfirming(true)
            else apply(false)
          }}
        />
      </label>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('buildTime.confirm.title', { key: secretKey })}
        description={t('buildTime.confirm.description')}
        confirmLabel={t('buildTime.confirm.confirm')}
        pendingLabel={t('buildTime.confirm.pending')}
        pending={mutation.isPending}
        onConfirm={() => {
          apply(true, () => {
            setConfirming(false)
          })
        }}
      />
    </>
  )
}
