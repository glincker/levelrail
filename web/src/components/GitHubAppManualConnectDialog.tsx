import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useConnectGitHubAppManually } from '../queries/githubApp'
import {
  FormDialogFooter,
  mutationToastCallbacks,
  ResettableDialog,
} from './ConnectionCard'

// ManualConnectDialog: an operator creates the App themselves at
// github.com/settings/apps/new and pastes the resulting credentials
// in directly. installation_id/account_login are optional: an App
// saved without them still connects (status.installed stays false).
export function ManualConnectDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const connect = useConnectGitHubAppManually()
  const [instanceURL, setInstanceURL] = useState('')
  const [appID, setAppID] = useState('')
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [webhookSecret, setWebhookSecret] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [installationID, setInstallationID] = useState('')
  const [accountLogin, setAccountLogin] = useState('')

  function resetForm() {
    setInstanceURL('')
    setAppID('')
    setClientID('')
    setClientSecret('')
    setWebhookSecret('')
    setPrivateKey('')
    setInstallationID('')
    setAccountLogin('')
  }

  const appIDNum = Number(appID)
  const canSubmit =
    appID.trim() !== '' &&
    Number.isFinite(appIDNum) &&
    appIDNum > 0 &&
    clientID.trim() !== '' &&
    clientSecret.trim() !== '' &&
    webhookSecret.trim() !== '' &&
    privateKey.trim() !== ''

  function handleSubmit() {
    if (!canSubmit) {
      return
    }
    const installationIDNum = installationID.trim()
      ? Number(installationID)
      : undefined
    connect.mutate(
      {
        app_id: appIDNum,
        client_id: clientID.trim(),
        instance_url: instanceURL.trim() || undefined,
        client_secret: clientSecret,
        webhook_secret: webhookSecret,
        private_key: privateKey,
        installation_id:
          installationIDNum !== undefined && Number.isFinite(installationIDNum)
            ? installationIDNum
            : undefined,
        account_login: accountLogin.trim() || undefined,
      },
      mutationToastCallbacks(
        'GitHub App connected.',
        'Could not connect the GitHub App.',
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
          <DialogTitle>Connect a GitHub App manually</DialogTitle>
          <DialogDescription>
            Create the App yourself at{' '}
            <a
              href="https://github.com/settings/apps/new"
              target="_blank"
              rel="noreferrer"
              className="underline"
            >
              github.com/settings/apps/new
            </a>{' '}
            (or your GitHub Enterprise Server instance&apos;s own
            /settings/apps/new), then paste the resulting credentials here. No
            primary domain required to save these: only automated setup needs
            one.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
          <Field>
            <FieldLabel htmlFor="gh-instance-url">
              GitHub instance (optional)
            </FieldLabel>
            <Input
              id="gh-instance-url"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              value={instanceURL}
              onChange={(e) => {
                setInstanceURL(e.target.value)
              }}
              placeholder="https://github.com"
            />
            <FieldDescription>
              Leave blank for github.com. Self-hosted instances work too, e.g.
              https://github.example.com.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-app-id">App ID</FieldLabel>
            <Input
              id="gh-app-id"
              type="number"
              inputMode="numeric"
              value={appID}
              onChange={(e) => {
                setAppID(e.target.value)
              }}
              placeholder="123456"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-client-id">Client ID</FieldLabel>
            <Input
              id="gh-client-id"
              value={clientID}
              onChange={(e) => {
                setClientID(e.target.value)
              }}
              placeholder="Iv1.abc123def456"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-client-secret">Client secret</FieldLabel>
            <Input
              id="gh-client-secret"
              type="password"
              value={clientSecret}
              onChange={(e) => {
                setClientSecret(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-webhook-secret">Webhook secret</FieldLabel>
            <Input
              id="gh-webhook-secret"
              type="password"
              value={webhookSecret}
              onChange={(e) => {
                setWebhookSecret(e.target.value)
              }}
            />
            <FieldDescription>
              The value you set as the App&apos;s own webhook secret on GitHub,
              not generated here.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-private-key">Private key (.pem)</FieldLabel>
            <Textarea
              id="gh-private-key"
              value={privateKey}
              onChange={(e) => {
                setPrivateKey(e.target.value)
              }}
              placeholder="-----BEGIN RSA PRIVATE KEY-----"
              rows={6}
              className="font-mono text-xs"
            />
            <FieldDescription>
              Generated once on the App&apos;s General page on GitHub and
              downloaded as a .pem file. Paste its full contents.
            </FieldDescription>
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field>
              <FieldLabel htmlFor="gh-installation-id">
                Installation ID (optional)
              </FieldLabel>
              <Input
                id="gh-installation-id"
                type="number"
                inputMode="numeric"
                value={installationID}
                onChange={(e) => {
                  setInstallationID(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="gh-account-login">
                Account login (optional)
              </FieldLabel>
              <Input
                id="gh-account-login"
                value={accountLogin}
                onChange={(e) => {
                  setAccountLogin(e.target.value)
                }}
                placeholder="octocat"
              />
            </Field>
          </div>
        </div>
        <FormDialogFooter
          onCancel={() => {
            resetForm()
            onOpenChange(false)
          }}
          submitDisabled={!canSubmit || connect.isPending}
          pending={connect.isPending}
          onSubmit={handleSubmit}
          submitLabel="Connect"
          pendingLabel="Connecting..."
        />
      </DialogContent>
    </ResettableDialog>
  )
}
