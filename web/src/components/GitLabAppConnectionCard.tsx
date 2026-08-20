import { useState } from 'react'
import {
  CheckCircleIcon,
  GitlabLogoIcon,
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
  useConnectGitLabApp,
  useDisconnectGitLabApp,
  useGitLabAppStatus,
} from '../queries/gitlabApp'

// Status card for the GitLab App connection: not connected / configured
// but not yet authorized / connected as <instance>. Unlike GitHub's
// manifest flow, GitLab has no programmatic App-creation API: the
// operator registers an OAuth Application themselves in their GitLab
// instance's own Applications settings and pastes the resulting
// instance URL, client ID, and client secret here (ConfigureDialog).
// "Connect" is then a separate, real browser navigation
// (window.location.href to GET /api/v1/gitlab-app/connect) that drives
// GitLab's own OAuth2 authorization prompt, the same "fetch cannot drive
// a redirect-based flow" reasoning GitHubAppConnectionCard.tsx's own doc
// comment gives for its own navigation.
export function GitLabAppConnectionCard() {
  const { data: status } = useGitLabAppStatus()
  const disconnect = useDisconnectGitLabApp()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [configureOpen, setConfigureOpen] = useState(false)

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GitlabLogoIcon className="size-4" />
          </div>
          <div>
            <CardTitle>GitLab App</CardTitle>
            <CardDescription>
              Connect a GitLab OAuth Application for project browsing and
              webhook-driven deploys, including self-hosted instances.
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
            {status.connected ? (
              <p className="font-mono text-sm text-muted-foreground">
                {status.instance_url}
              </p>
            ) : null}
            {status.connected && !status.authorized ? (
              <p className="text-sm text-muted-foreground">
                The OAuth Application is configured but hasn&apos;t been
                authorized yet. Click Connect to finish.
              </p>
            ) : null}
          </div>

          {status.connected ? (
            <div className="flex shrink-0 items-center gap-2">
              {!status.authorized ? (
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    window.location.href = '/api/v1/gitlab-app/connect'
                  }}
                >
                  <GitlabLogoIcon className="size-4" />
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
                      Disconnect GitLab App?
                    </DialogTitle>
                    <DialogDescription>
                      This stops this control plane from using the connection to
                      list projects or register webhooks. It does not revoke the
                      authorization or delete the Application on GitLab itself.
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
                            toast.add({ title: 'GitLab App disconnected.', type: 'success' })
                          },
                          onError: (error) => {
                            toast.add({
                              title: 'Could not disconnect the GitLab App.',
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
              <GitlabLogoIcon className="size-4" />
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

// ConfigureDialog saves the OAuth Application's own instance/client_id/
// client_secret (PUT /api/v1/gitlab-app). The operator creates this
// Application themselves at <instance>/-/profile/applications (or, for
// an instance-wide app, admin/applications) with redirect_uri
// <this control plane's base url>/api/v1/gitlab-app/callback and scope
// "api", then pastes the resulting client ID/secret here.
function ConfigureDialog({
  open,
  onOpenChange,
  baseURL,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  baseURL?: string
}) {
  const connect = useConnectGitLabApp()
  const [instanceURL, setInstanceURL] = useState('https://gitlab.com')
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')

  function resetForm() {
    setInstanceURL('https://gitlab.com')
    setClientID('')
    setClientSecret('')
  }

  const canSubmit =
    instanceURL.trim() !== '' && clientID.trim() !== '' && clientSecret.trim() !== ''

  function handleSubmit() {
    if (!canSubmit) {
      return
    }
    connect.mutate(
      {
        instance_url: instanceURL.trim(),
        client_id: clientID.trim(),
        client_secret: clientSecret,
      },
      {
        onSuccess: () => {
          toast.add({ title: 'GitLab App configured.', type: 'success' })
          resetForm()
          onOpenChange(false)
        },
        onError: (error) => {
          toast.add({
            title: 'Could not configure the GitLab App.',
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
          <DialogTitle>Configure a GitLab OAuth Application</DialogTitle>
          <DialogDescription>
            Create one in your GitLab instance under Applications, with
            redirect URI{' '}
            {baseURL ? (
              <code className="text-xs">{baseURL}/api/v1/gitlab-app/callback</code>
            ) : (
              <span className="text-amber-700 dark:text-amber-400">
                set a primary domain in domain settings first
              </span>
            )}{' '}
            and scope <code className="text-xs">api</code>, then paste the
            resulting client ID and secret here.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="gl-instance-url">Instance URL</FieldLabel>
            <Input
              id="gl-instance-url"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              value={instanceURL}
              onChange={(e) => {
                setInstanceURL(e.target.value)
              }}
              placeholder="https://gitlab.com"
            />
            <FieldDescription>
              Self-hosted instances work too, e.g.
              https://gitlab.internal.example.com.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="gl-client-id">Application ID</FieldLabel>
            <Input
              id="gl-client-id"
              autoComplete="off"
              spellCheck={false}
              value={clientID}
              onChange={(e) => {
                setClientID(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gl-client-secret">Secret</FieldLabel>
            <Input
              id="gl-client-secret"
              type="password"
              autoComplete="off"
              spellCheck={false}
              value={clientSecret}
              onChange={(e) => {
                setClientSecret(e.target.value)
              }}
            />
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
