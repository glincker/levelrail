import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  HardDrivesIcon,
  CheckCircleIcon,
  WarningCircleIcon,
  MinusCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import type { RegistrySettings } from '../queries/registry'
import {
  useDisableRegistry,
  useUpdateRegistrySettings,
} from '../queries/registry'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

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

const STATUS_ICON: Record<RegistrySettings['status'], Icon> = {
  running: CheckCircleIcon,
  error: WarningCircleIcon,
  stopped: MinusCircleIcon,
}

function StatusBadge({ status }: { status: RegistrySettings['status'] }) {
  const StatusIcon = STATUS_ICON[status]
  return (
    <Badge variant={STATUS_VARIANT[status]} className="shrink-0">
      <StatusIcon className="size-3" />
      {STATUS_LABEL[status]}
    </Badge>
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
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HardDrivesIcon className="size-4" />
          Container registry
          <StatusBadge status={settings.status} />
        </CardTitle>
        <CardDescription>
          Levelrail&apos;s own built-in image registry: a build cache and
          distribution backend for multi-node deployments with no external
          registry to sign up for. Fronted by TLS through the embedded
          ingress; the registry enforces its own login, so credentials are
          required even if the published port is reached directly.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="space-y-5"
        >
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-foreground">Enabled</p>
              <p className="text-sm text-muted-foreground">
                Runs the registry container and routes it through the
                configured host.
              </p>
            </div>
            <Switch
              checked={enabled}
              onCheckedChange={setEnabled}
              disabled={pending}
              aria-label="Container registry enabled"
            />
          </div>

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
              For a single node, any hostname that resolves to this
              machine works. For multi-node, use a WireGuard mesh-resolvable
              name so every node can reach it.
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
            <div className="space-y-2">
              <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                <WarningIcon className="mt-0.5 size-4 shrink-0" />
                <p className="text-sm">
                  Copy this password now. It will not be shown again.
                </p>
              </div>
              <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                  {revealedPassword}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={copyPassword}
                >
                  {copied ? <CheckIcon /> : <CopyIcon />}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                docker login {host || 'HOST'} -u {settings.username ?? 'levelrail'}
              </p>
            </div>
          ) : null}

          {settings.status === 'error' && settings.message ? (
            <Alert variant="destructive">
              <AlertDescription>{settings.message}</AlertDescription>
            </Alert>
          ) : null}
          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          {updateSettings.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateSettings.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {disable.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{disable.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
              {updateSettings.isPending ? 'Saving...' : 'Save'}
            </Button>
            {(settings.enabled || settings.has_credentials) && (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={handleDisable}
              >
                {disable.isPending ? 'Disabling...' : 'Disable'}
              </Button>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
