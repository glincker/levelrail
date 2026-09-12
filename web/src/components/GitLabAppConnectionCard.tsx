import { useState } from 'react'
import { GitlabLogoIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  useConnectGitLabApp,
  useDisconnectGitLabApp,
  useGitLabAppStatus,
} from '../queries/gitlabApp'
import { SetPrimaryDomainPrompt } from './SetPrimaryDomainPrompt'
import {
  ConfiguredStatusHeading,
  ConnectionCardHeader,
  DisconnectConnectionDialog,
  FormDialogFooter,
  mutationToastCallbacks,
  ResettableDialog,
} from './ConnectionCard'

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
      <ConnectionCardHeader
        icon={GitlabLogoIcon}
        title="GitLab App"
        description="Connect a GitLab OAuth Application for project browsing and webhook-driven deploys, including self-hosted instances."
      />
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
          <div className="space-y-1">
            <ConfiguredStatusHeading
              connected={status.connected}
              authorized={status.authorized}
            />
            {status.connected ? (
              <p className="font-mono text-sm text-muted-foreground">
                {status.instance_url}
              </p>
            ) : null}
            {status.connected && !status.authorized && status.base_url ? (
              <p className="text-sm text-muted-foreground">
                The OAuth Application is configured but hasn&apos;t been
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
                    window.location.href = '/api/v1/gitlab-app/connect'
                  }}
                >
                  <GitlabLogoIcon className="size-4" />
                  Connect
                </Button>
              ) : null}
              <DisconnectConnectionDialog
                open={confirmOpen}
                onOpenChange={setConfirmOpen}
                title="Disconnect GitLab App?"
                description={
                  <>
                    This stops this control plane from using the connection to
                    list projects or register webhooks. It does not revoke the
                    authorization or delete the Application on GitLab itself.
                  </>
                }
                pending={disconnect.isPending}
                onConfirm={() => {
                  disconnect.mutate(
                    undefined,
                    mutationToastCallbacks(
                      'GitLab App disconnected.',
                      'Could not disconnect the GitLab App.',
                      () => setConfirmOpen(false),
                    ),
                  )
                }}
              />
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
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  baseURL?: string
}>) {
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
      mutationToastCallbacks(
        'GitLab App configured.',
        'Could not configure the GitLab App.',
        () => {
          resetForm()
          onOpenChange(false)
        },
      ),
    )
  }

  return (
    <ResettableDialog open={open} onOpenChange={onOpenChange} onReset={resetForm}>
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
        <FormDialogFooter
          onCancel={() => {
            resetForm()
            onOpenChange(false)
          }}
          submitDisabled={!canSubmit || connect.isPending}
          pending={connect.isPending}
          onSubmit={handleSubmit}
          submitLabel="Save"
          pendingLabel="Saving..."
        />
      </DialogContent>
    </ResettableDialog>
  )
}
