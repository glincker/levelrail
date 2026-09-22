import { useState } from 'react'
import { TeaBagIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  useConnectGiteaApp,
  useDisconnectGiteaApp,
  useGiteaAppStatus,
} from '../queries/giteaApp'
import { SetPrimaryDomainPrompt } from './SetPrimaryDomainPrompt'
import {
  ConfiguredStatusHeading,
  ConnectionCardHeader,
  DisconnectConnectionDialog,
  FormDialogFooter,
  mutationToastCallbacks,
  ResettableDialog,
} from './ConnectionCard'

// Status card for the Gitea App connection: not connected / configured
// but not yet authorized / connected as <instance>. Mirrors
// GitLabAppConnectionCard.tsx exactly: Gitea, like GitLab, has no
// programmatic App-creation API, so the operator registers an OAuth2
// application themselves in their Gitea instance's own Applications
// settings and pastes the resulting instance URL, client ID, and client
// secret here (ConfigureDialog). "Connect" is then a separate, real
// browser navigation (window.location.href to
// GET /api/v1/gitea-app/connect) that drives Gitea's own OAuth2
// authorization prompt.
export function GiteaAppConnectionCard() {
  const { data: status } = useGiteaAppStatus()
  const disconnect = useDisconnectGiteaApp()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [configureOpen, setConfigureOpen] = useState(false)

  return (
    <Card>
      <ConnectionCardHeader
        icon={TeaBagIcon}
        title="Gitea App"
        description="Connect a Gitea OAuth2 application for repo browsing and webhook-driven deploys, including self-hosted instances."
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
                The OAuth2 application is configured but hasn&apos;t been
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
                    window.location.href = '/api/v1/gitea-app/connect'
                  }}
                >
                  <TeaBagIcon className="size-4" />
                  Connect
                </Button>
              ) : null}
              <DisconnectConnectionDialog
                open={confirmOpen}
                onOpenChange={setConfirmOpen}
                title="Disconnect Gitea App?"
                description={
                  <>
                    This stops this control plane from using the connection to
                    list repos or register webhooks. It does not revoke the
                    authorization or delete the application on Gitea itself.
                  </>
                }
                pending={disconnect.isPending}
                onConfirm={() => {
                  disconnect.mutate(
                    undefined,
                    mutationToastCallbacks(
                      'Gitea App disconnected.',
                      'Could not disconnect the Gitea App.',
                      () => setConfirmOpen(false),
                    ),
                  )
                }}
              />
            </div>
          ) : (
            <Button
              type="button"
              size="sm"
              onClick={() => setConfigureOpen(true)}
            >
              <TeaBagIcon className="size-4" />
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

// ConfigureDialog saves the OAuth2 application's own instance/client_id/
// client_secret (PUT /api/v1/gitea-app). The operator creates this
// application themselves at <instance>/user/settings/applications with
// redirect URI <this control plane's base url>/api/v1/gitea-app/callback,
// then pastes the resulting client ID/secret here.
function ConfigureDialog({
  open,
  onOpenChange,
  baseURL,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  baseURL?: string
}>) {
  const connect = useConnectGiteaApp()
  const [instanceURL, setInstanceURL] = useState('')
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')

  function resetForm() {
    setInstanceURL('')
    setClientID('')
    setClientSecret('')
  }

  const canSubmit =
    instanceURL.trim() !== '' &&
    clientID.trim() !== '' &&
    clientSecret.trim() !== ''

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
        'Gitea App configured.',
        'Could not configure the Gitea App.',
        () => {
          resetForm()
          onOpenChange(false)
        },
      ),
    )
  }

  return (
    <ResettableDialog
      open={open}
      onOpenChange={onOpenChange}
      onReset={resetForm}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Configure a Gitea OAuth2 Application</DialogTitle>
          <DialogDescription>
            Create one in your Gitea instance under Settings &gt; Applications,
            with redirect URI{' '}
            {baseURL ? (
              <code className="text-xs">
                {baseURL}/api/v1/gitea-app/callback
              </code>
            ) : (
              <span className="text-amber-700 dark:text-amber-400">
                set a primary domain in domain settings first
              </span>
            )}
            , then paste the resulting client ID and secret here.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="gitea-instance-url">Instance URL</FieldLabel>
            <Input
              id="gitea-instance-url"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              value={instanceURL}
              onChange={(e) => {
                setInstanceURL(e.target.value)
              }}
              placeholder="https://git.example.com"
            />
            <FieldDescription>
              Gitea is almost always self-hosted, e.g.
              https://git.internal.example.com.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="gitea-client-id">Client ID</FieldLabel>
            <Input
              id="gitea-client-id"
              autoComplete="off"
              spellCheck={false}
              value={clientID}
              onChange={(e) => {
                setClientID(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gitea-client-secret">Client Secret</FieldLabel>
            <Input
              id="gitea-client-secret"
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
