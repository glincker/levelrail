import { useState, type FormEvent } from 'react'
import type { UseMutationResult } from '@tanstack/react-query'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import type { ApiError } from '../lib/apiError'

interface TokenSettings {
  enabled: boolean
  has_token: boolean
}

interface TokenSettingsRequest {
  enabled: boolean
  token?: string
}

// Shared by the two instance-level Cloudflare credential cards (Tunnel
// connector token, DNS-01 API token): same enabled/token/formError
// shape, same "token required unless one is already stored" validation,
// same save/disconnect mutation flow.
// eslint-disable-next-line react-refresh/only-export-components
export function useTokenToggleForm<TSettings extends TokenSettings>({
  settings,
  updateMutation,
  disconnectMutation,
  requiredTokenMessage,
  saveSuccessTitle,
  disconnectSuccessTitle,
}: Readonly<{
  settings: TSettings
  updateMutation: UseMutationResult<TSettings, ApiError, TokenSettingsRequest>
  disconnectMutation: UseMutationResult<TSettings, ApiError, void>
  requiredTokenMessage: string
  saveSuccessTitle: string
  disconnectSuccessTitle: string
}>) {
  const [enabled, setEnabled] = useState(settings.enabled)
  const [token, setToken] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (enabled && !settings.has_token && !token.trim()) {
      setFormError(requiredTokenMessage)
      return
    }
    updateMutation.mutate(
      { enabled, token: token.trim() || undefined },
      {
        onSuccess: () => {
          setToken('')
          toast.add({ title: saveSuccessTitle, type: 'success' })
        },
      },
    )
  }

  function handleDisconnect() {
    disconnectMutation.mutate(undefined, {
      onSuccess: () => {
        setEnabled(false)
        setToken('')
        toast.add({ title: disconnectSuccessTitle, type: 'success' })
      },
    })
  }

  return {
    enabled,
    setEnabled,
    token,
    setToken,
    formError,
    handleSubmit,
    handleDisconnect,
    pending: updateMutation.isPending || disconnectMutation.isPending,
  }
}

export function SettingsEnabledRow({
  description,
  checked,
  onCheckedChange,
  disabled,
  ariaLabel,
}: Readonly<{
  description: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled: boolean
  ariaLabel: string
}>) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <p className="text-sm font-medium text-foreground">Enabled</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        aria-label={ariaLabel}
      />
    </div>
  )
}

export function SettingsFormAlerts({
  alerts,
}: Readonly<{
  alerts: ReadonlyArray<{ key: string; message: string } | null | undefined | false>
}>) {
  return (
    <>
      {alerts.map((alert) =>
        alert ? (
          <Alert variant="destructive" key={alert.key}>
            <AlertDescription>{alert.message}</AlertDescription>
          </Alert>
        ) : null,
      )}
    </>
  )
}

export function SettingsFormActions({
  pending,
  savePending,
  showSecondary,
  secondaryPending,
  secondaryLabel,
  secondaryPendingLabel,
  onSecondaryClick,
}: Readonly<{
  pending: boolean
  savePending: boolean
  showSecondary: boolean
  secondaryPending: boolean
  secondaryLabel: string
  secondaryPendingLabel: string
  onSecondaryClick: () => void
}>) {
  return (
    <div className="flex items-center gap-2">
      <Button type="submit" size="sm" disabled={pending}>
        {savePending ? 'Saving...' : 'Save'}
      </Button>
      {showSecondary && (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={pending}
          onClick={onSecondaryClick}
        >
          {secondaryPending ? secondaryPendingLabel : secondaryLabel}
        </Button>
      )}
    </div>
  )
}
