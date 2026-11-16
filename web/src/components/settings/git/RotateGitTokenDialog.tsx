import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  defaultRotateExpiry,
  ROTATE_DATE_PATTERN,
} from '../../../lib/gitCredential'
import { useRotateGitConnection } from '../../../queries/gitConnections'
import { useRuntimeSettings } from '../../../queries/runtimeSettings'
import type { GitProviderConnection } from '../../../types/gitConnections'

const LIFETIME_KEY = 'git.rotate_lifetime_days'
const FALLBACK_LIFETIME_DAYS = 90
const DATE_PLACEHOLDER = 'YYYY-MM-DD'

// GitLab issues a new token and the old one stops at once, so the dialog says
// so before anything is sent. The new token itself never reaches the browser.
export function RotateGitTokenDialog({
  connection,
  open,
  onOpenChange,
}: Readonly<{
  connection: GitProviderConnection
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('gitConnect')
  const rotate = useRotateGitConnection()
  const { data } = useRuntimeSettings()
  const [now] = useState(() => Date.now())
  const [typed, setTyped] = useState<string | null>(null)
  const inputId = useId()

  const setting = Array.isArray(data?.settings)
    ? data.settings.find((s) => s.key === LIFETIME_KEY)
    : undefined
  const lifetime = Number(setting?.value)
  const days =
    Number.isInteger(lifetime) && lifetime > 0
      ? lifetime
      : FALLBACK_LIFETIME_DAYS
  const date = typed ?? defaultRotateExpiry(days, now)
  const valid = date === '' || ROTATE_DATE_PATTERN.test(date)

  function errorText(): string | null {
    const e = rotate.error
    if (!e) return null
    switch (e.status) {
      case 501:
        return t('rotate.errors.noMasterKey')
      case 502:
        return t('rotate.errors.provider')
      case 400:
        return t('rotate.errors.badDate')
      default:
        return e.message
    }
  }

  function close(next: boolean) {
    if (!next) {
      rotate.reset()
      setTyped(null)
    }
    onOpenChange(next)
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={close}
      title={t('rotate.title', { label: connection.label })}
      description={t('rotate.description')}
      consequences={[t('rotate.consequence')]}
      confirmLabel={t('rotate.confirm')}
      pendingLabel={t('rotate.pending')}
      pending={rotate.isPending}
      confirmDisabled={!valid}
      error={errorText()}
      focusCancel
      onConfirm={() =>
        rotate.mutate(
          { id: connection.id, expiresAt: date || undefined },
          {
            onSuccess: (res) => {
              close(false)
              toast.add({
                title: t('rotate.toast.done'),
                description: res.expires_at
                  ? t('rotate.toast.expires', {
                      date: new Date(res.expires_at).toLocaleDateString(),
                    })
                  : t('rotate.toast.noExpiry'),
                type: 'success',
              })
            },
          },
        )
      }
    >
      <div className="space-y-1.5">
        <Label htmlFor={inputId}>{t('rotate.expiryLabel')}</Label>
        <Input
          id={inputId}
          value={date}
          inputMode="numeric"
          placeholder={DATE_PLACEHOLDER}
          autoComplete="off"
          aria-invalid={!valid}
          className="font-mono"
          onChange={(event) => setTyped(event.target.value.trim())}
        />
        <p className="text-xs text-muted-foreground">
          {t('rotate.expiryHint', { days })}
        </p>
      </div>
    </ConfirmDialog>
  )
}
