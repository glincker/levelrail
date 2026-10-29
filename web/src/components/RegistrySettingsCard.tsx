import { useState } from 'react'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CopyGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import type { VariantProps } from 'class-variance-authority'
import type { RegistrySettings } from '../queries/registry'
import {
  useDisableRegistry,
  useUpdateRegistrySettings,
} from '../queries/registry'
import type { badgeVariants } from '@/components/ui/badge'
import { StatusBadge } from '@/components/ui/status-badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  SettingsEnabledRow,
  SettingsFormActions,
  SettingsFormAlerts,
} from './SettingsCard'

const STATUS_VARIANT: Record<
  RegistrySettings['status'],
  VariantProps<typeof badgeVariants>['variant']
> = {
  running: 'success',
  error: 'destructive',
  stopped: 'muted',
}

const STATUS_LABEL: Record<RegistrySettings['status'], string> = {
  running: 'Running',
  error: 'Error',
  stopped: 'Stopped',
}

const STATUS_ICON: Record<RegistrySettings['status'], UiIcon> = {
  running: CircleCheckGlassIcon,
  error: CircleAlertGlassIcon,
  stopped: CircleMinusGlassIcon,
}

export function RegistryStatusBadge({
  status,
}: {
  status: RegistrySettings['status']
}) {
  return (
    <StatusBadge
      variant={STATUS_VARIANT[status]}
      label={STATUS_LABEL[status]}
      icon={STATUS_ICON[status]}
    />
  )
}

// Instance-level built-in container registry: GET/PUT/DELETE
// /api/v1/settings/registry. One registry per control plane (not
// per-app), matching store.RegistrySettings' own single-row shape. The
// password is platform-generated and paste-once: it only ever appears in
// the response to the PUT call that generates it (the first enable),
// mirroring CreateTokenDialog's own "copy this now, it will not be shown
// again" treatment for a comparable platform-generated secret.
export function RegistrySettingsCard({
  settings,
}: {
  settings: RegistrySettings
}) {
  const [enabled, setEnabled] = useState(settings.enabled)
  const [host, setHost] = useState(settings.host ?? '')
  const [formError, setFormError] = useState<string | null>(null)
  const [revealedPassword, setRevealedPassword] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const updateSettings = useUpdateRegistrySettings()
  const disable = useDisableRegistry()

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    setRevealedPassword(null)
    if (enabled && !host.trim()) {
      setFormError('A hostname is required to enable the built-in registry.')
      return
    }
    updateSettings.mutate(
      { enabled, host: host.trim() || undefined },
      {
        onSuccess: (updated) => {
          toast.add({ title: 'Registry settings saved.', type: 'success' })
          if (updated.password) {
            setRevealedPassword(updated.password)
          }
        },
      },
    )
  }

  function handleDisable() {
    disable.mutate(undefined, {
      onSuccess: () => {
        setEnabled(false)
        setHost('')
        setRevealedPassword(null)
        toast.add({ title: 'Registry disabled.', type: 'success' })
      },
    })
  }

  function copyPassword() {
    if (!revealedPassword) {
      return
    }
    void navigator.clipboard.writeText(revealedPassword).then(() => {
      setCopied(true)
    })
  }

  const pending = updateSettings.isPending || disable.isPending

  return (
    <Card>
      <CardContent>
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="space-y-4"
        >
          <SettingsEnabledRow
            description="Runs the registry container and routes it through the configured host."
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={pending}
            ariaLabel="Container registry enabled"
          />

          <Field>
            <FieldLabel htmlFor="registry-host">Host</FieldLabel>
            <Input
              id="registry-host"
              value={host}
              onChange={(e) => {
                setHost(e.target.value)
              }}
              disabled={pending}
              placeholder="registry.internal.example.com"
            />
            <FieldDescription>
              For a single node, any hostname that resolves to this machine
              works. For multi-node, use a WireGuard mesh-resolvable name so
              every node can reach it.
            </FieldDescription>
          </Field>

          {settings.has_credentials ? (
            <p className="text-sm text-muted-foreground">
              Username: <code className="text-xs">{settings.username}</code>.
              Password was shown once when the registry was first enabled.
              Disable and re-enable to generate a new one.
            </p>
          ) : null}

          {revealedPassword ? (
            <div className="space-y-4">
              <Banner variant="warning" title="Copy this password now.">
                It will not be shown again.
              </Banner>
              <div className="flex items-center gap-2 rounded-lg bg-muted/50 p-2">
                <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                  {revealedPassword}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={copyPassword}
                >
                  {copied ? (
                    <CheckGlassIcon size="1.2em" />
                  ) : (
                    <CopyGlassIcon size="1.2em" />
                  )}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                docker login {host || 'HOST'} -u{' '}
                {settings.username ?? 'levelrail'}
              </p>
            </div>
          ) : null}

          <SettingsFormAlerts
            alerts={[
              settings.status === 'error' && settings.message
                ? { key: 'status', message: settings.message }
                : null,
              formError ? { key: 'form', message: formError } : null,
              updateSettings.isError
                ? { key: 'update', message: updateSettings.error.message }
                : null,
              disable.isError
                ? { key: 'disable', message: disable.error.message }
                : null,
            ]}
          />

          <SettingsFormActions
            pending={pending}
            savePending={updateSettings.isPending}
            showSecondary={settings.enabled || settings.has_credentials}
            secondaryPending={disable.isPending}
            secondaryLabel="Disable"
            secondaryPendingLabel="Disabling..."
            onSecondaryClick={handleDisable}
          />
        </form>
      </CardContent>
    </Card>
  )
}
