import { useState } from 'react'
import { GitBranchIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  useConnectBitbucketApp,
  useDisconnectBitbucketApp,
  useBitbucketAppStatus,
} from '../queries/bitbucketApp'
import { SetPrimaryDomainPrompt } from './SetPrimaryDomainPrompt'
import {
  ConfiguredStatusHeading,
  ConnectionCardHeader,
  DisconnectConnectionDialog,
  FormDialogFooter,
  mutationToastCallbacks,
  ResettableDialog,
} from './ConnectionCard'

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
      <ConnectionCardHeader
        icon={GitBranchIcon}
        title="Bitbucket App"
        description="Connect a Bitbucket Cloud OAuth consumer for repository browsing and webhook-driven deploys."
      />
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
          <div className="space-y-1">
            <ConfiguredStatusHeading
              connected={status.connected}
              authorized={status.authorized}
            />
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
              <DisconnectConnectionDialog
                open={confirmOpen}
                onOpenChange={setConfirmOpen}
                title="Disconnect Bitbucket App?"
                description={
                  <>
                    This stops this control plane from using the connection to
                    list repositories or register webhooks. It does not revoke
                    the authorization or delete the consumer on Bitbucket
                    itself.
                  </>
                }
                pending={disconnect.isPending}
                onConfirm={() => {
                  disconnect.mutate(
                    undefined,
                    mutationToastCallbacks(
                      'Bitbucket App disconnected.',
                      'Could not disconnect the Bitbucket App.',
                      () => setConfirmOpen(false),
                    ),
                  )
                }}
              />
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
      mutationToastCallbacks(
        'Bitbucket App configured.',
        'Could not configure the Bitbucket App.',
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
