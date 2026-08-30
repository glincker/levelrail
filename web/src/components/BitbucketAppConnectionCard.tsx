import { useState } from 'react'
import {
  CheckCircleIcon,
  GitBranchIcon,
  WarningIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import {
  useConnectBitbucketApp,
  useDisconnectBitbucketApp,
  useBitbucketAppStatus,
} from '../queries/bitbucketApp'
import { SetPrimaryDomainPrompt } from './SetPrimaryDomainPrompt'

// Status card for the Bitbucket App connection: not connected /
// configured but not yet authorized / connected. Cloud only, no
// instance URL field (docs/design/git-provider-integrations.md section
// 3): Bitbucket Server has no OAuth-consumer equivalent. Like GitLab,
// Bitbucket has no programmatic consumer-creation API: the operator
// registers an OAuth consumer themselves in their workspace settings
// and pastes the resulting key/secret here (ConfigureDialog). "Connect"
// is then a separate, real browser navigation (window.location.href to
// GET /api/v1/bitbucket-app/connect) that drives Bitbucket's own OAuth2
// authorization prompt, the same shape GitLabAppConnectionCard.tsx's
// own doc comment describes.
export function BitbucketAppConnectionCard() {
  const { data: status } = useBitbucketAppStatus()
  const disconnect = useDisconnectBitbucketApp()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [configureOpen, setConfigureOpen] = useState(false)

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GitBranchIcon className="size-4" />
          </div>
          <div>
            <CardTitle>Bitbucket App</CardTitle>
            <CardDescription>
              Connect a Bitbucket Cloud OAuth consumer for repository
              browsing and webhook-driven deploys.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
          <div className="space-y-1">
            {status.connected ? (
              <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                <CheckCircleIcon className="size-4 text-green-600 dark:text-green-400" />
                Configured
                {status.authorized ? (
                  <Badge variant="success">authorized</Badge>
                ) : (
                  <Badge variant="warning">not authorized yet</Badge>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
                <XCircleIcon className="size-4" />
                Not connected
              </div>
            )}
            {status.connected && !status.authorized && status.base_url ? (
              <p className="text-sm text-muted-foreground">
                The OAuth consumer is configured but hasn&apos;t been
                authorized yet. Click Connect to finish.
              </p>
            ) : null}
            {status.connected && !status.authorized && !status.base_url ? (
              <SetPrimaryDomainPrompt />
            ) : null}
          </div>

          {status.connected ? (
            <div className="flex shrink-0 items-center gap-2">
              {!status.authorized && status.base_url ? (
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    window.location.href = '/api/v1/bitbucket-app/connect'
                  }}
                >
                  <GitBranchIcon className="size-4" />
                  Connect
                </Button>
              ) : null}
              <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                <DialogTrigger render={<Button variant="destructive" size="sm" />}>
                  Disconnect
                </DialogTrigger>
                <DialogContent className="sm:max-w-sm">
                  <DialogHeader>
                    <DialogTitle className="flex items-center gap-1.5 text-destructive">
                      <WarningIcon className="size-4" aria-hidden="true" />
                      Disconnect Bitbucket App?
                    </DialogTitle>
                    <DialogDescription>
                      This stops this control plane from using the connection to
                      list repositories or register webhooks. It does not revoke
                      the authorization or delete the consumer on Bitbucket
                      itself.
                    </DialogDescription>
                  </DialogHeader>
                  <DialogFooter>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setConfirmOpen(false)}
                    >
                      Cancel
                    </Button>
                    <Button
                      type="button"
                      variant="destructive"
                      disabled={disconnect.isPending}
                      onClick={() => {
                        disconnect.mutate(undefined, {
                          onSuccess: () => {
                            setConfirmOpen(false)
                            toast.add({ title: 'Bitbucket App disconnected.', type: 'success' })
                          },
                          onError: (error) => {
                            toast.add({
                              title: 'Could not disconnect the Bitbucket App.',
                              description: error.message,
                              type: 'error',
                            })
                          },
                        })
                      }}
                    >
                      {disconnect.isPending ? 'Disconnecting...' : 'Disconnect'}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>
          ) : (
            <Button type="button" size="sm" onClick={() => setConfigureOpen(true)}>
              <GitBranchIcon className="size-4" />
              Configure
            </Button>
          )}
        </div>
      </CardContent>
      <ConfigureDialog
        open={configureOpen}
        onOpenChange={setConfigureOpen}
        baseURL={status.base_url}
      />
    </Card>
  )
}

// ConfigureDialog saves the OAuth consumer's own key/secret
// (PUT /api/v1/bitbucket-app). The operator creates this consumer
// themselves at bitbucket.org, workspace settings > OAuth consumers,
// with the callback URL below and, since Bitbucket scopes permissions
// on the consumer itself rather than requesting them per authorization
// (unlike GitLab's own "api" scope), Account: Read, Repositories: Read,
// and Webhooks: Read and Write ticked.
function ConfigureDialog({
  open,
  onOpenChange,
  baseURL,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  baseURL?: string
}>) {
  const connect = useConnectBitbucketApp()
  const [key, setKey] = useState('')
  const [secret, setSecret] = useState('')

  function resetForm() {
    setKey('')
    setSecret('')
  }

  const canSubmit = key.trim() !== '' && secret.trim() !== ''

  function handleSubmit() {
    if (!canSubmit) {
      return
    }
    connect.mutate(
      {
        key: key.trim(),
        secret,
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Bitbucket App configured.', type: 'success' })
          resetForm()
          onOpenChange(false)
        },
        onError: (error) => {
          toast.add({
            title: 'Could not configure the Bitbucket App.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          resetForm()
        }
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Configure a Bitbucket OAuth consumer</DialogTitle>
          <DialogDescription>
            Create one at bitbucket.org under workspace settings &gt; OAuth
            consumers, with callback URL{' '}
            {baseURL ? (
              <code className="text-xs">{baseURL}/api/v1/bitbucket-app/callback</code>
            ) : (
              <span className="text-amber-700 dark:text-amber-400">
                set a primary domain in domain settings first
              </span>
            )}
            . Tick Account: Read, Repositories: Read, and Webhooks: Read and
            Write, then paste the resulting key and secret here.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="bb-key">Key</FieldLabel>
            <Input
              id="bb-key"
              autoComplete="off"
              spellCheck={false}
              value={key}
              onChange={(e) => {
                setKey(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="bb-secret">Secret</FieldLabel>
            <Input
              id="bb-secret"
              type="password"
              autoComplete="off"
              spellCheck={false}
              value={secret}
              onChange={(e) => {
                setSecret(e.target.value)
              }}
            />
            <FieldDescription>
              Shown once when the consumer is created. If you&apos;ve lost it,
              regenerate it on Bitbucket and paste the new value here.
            </FieldDescription>
          </Field>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              resetForm()
              onOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={!canSubmit || connect.isPending}
            onClick={handleSubmit}
          >
            {connect.isPending ? 'Saving...' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
