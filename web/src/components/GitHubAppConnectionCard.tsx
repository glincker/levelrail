import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  GithubLogoIcon,
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
import { Textarea } from '@/components/ui/textarea'
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
import { useIngressSettings } from '../queries/domains'
import {
  useConnectGitHubAppManually,
  useDisconnectGitHubApp,
  useGitHubAppStatus,
} from '../queries/githubApp'

// Status card for the GitHub App connection: not connected / connected
// as <account> but not yet installed / connected and installed on
// <account>. The "Add GitHub App" action is a real, full-page browser
// navigation (window.location.href to
// GET /api/v1/github-app/register/start), not a fetch call:
// GitHub's manifest flow is inherently a form POST from the browser to
// github.com, followed by GitHub's own confirmation page, followed by a
// redirect back here (internal/api/github_app_register.go's own doc
// comment covers this in full). A fetch/XHR call cannot drive that
// sequence; only a real navigation can.
//
// Two connect paths, side by side, matching Coolify's own GitHub App
// screen (Automated vs Manual installation): the automated manifest
// flow needs a real, publicly reachable primary domain, since GitHub's
// own servers redirect there for the lifetime of the App registration,
// not just for this browser session (see githubAppBaseURL's own doc
// comment on the backend). An operator whose domain isn't live yet, or
// who prefers creating the App by hand on github.com, uses the manual
// dialog instead: same end state (a connected App), no domain required
// to save the credentials, since ManualConnectDialog itself never talks
// to GitHub, it only stores what the operator already has.
export function GitHubAppConnectionCard() {
  const { data: status } = useGitHubAppStatus()
  const { data: ingressSettings } = useIngressSettings()
  const disconnect = useDisconnectGitHubApp()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [manualOpen, setManualOpen] = useState(false)

  const hasPrimaryDomain = Boolean(ingressSettings.primary_domain)
  const baseURL = status.base_url ?? ''

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GithubLogoIcon className="size-4" />
          </div>
          <div>
            <CardTitle>GitHub App</CardTitle>
            <CardDescription>
              Connect a GitHub App for private-repository access and
              installation-based repo browsing.
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
                Connected
                {status.installed && status.account_login ? (
                  <Badge variant="success">as {status.account_login}</Badge>
                ) : (
                  <Badge variant="warning">not installed yet</Badge>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
                <XCircleIcon className="size-4" />
                Not connected
              </div>
            )}
            {status.connected && !status.installed ? (
              <p className="text-sm text-muted-foreground">
                The App was created but hasn&apos;t been installed on an account
                or organization yet. Finish installing it on GitHub to pick
                repositories.
              </p>
            ) : null}
            {!status.connected && !hasPrimaryDomain ? (
              <p className="flex items-center gap-1.5 text-sm text-amber-700 dark:text-amber-400">
                <WarningIcon className="size-3.5" />
                Set a primary domain in{' '}
                <Link to="/domains" className="underline">
                  domain settings
                </Link>{' '}
                first for automated setup, or connect manually below.
              </p>
            ) : null}
            {!status.connected && hasPrimaryDomain && baseURL ? (
              <p className="flex items-start gap-1.5 text-sm text-amber-700 dark:text-amber-400">
                <WarningIcon className="mt-0.5 size-3.5 shrink-0" />
                <span>
                  Automated setup registers a callback at{' '}
                  <code className="font-mono">{baseURL}</code>. GitHub redirects
                  there for the life of the App, so this instance must actually
                  be publicly reachable at that address before you continue, not
                  just wherever you&apos;re viewing this page from right now.
                  Change it in{' '}
                  <Link to="/domains" className="underline">
                    domain settings
                  </Link>{' '}
                  if that&apos;s not where this instance actually lives, or use{' '}
                  manual setup instead. See GitHub&apos;s own{' '}
                  <a
                    href="https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app-from-a-manifest"
                    target="_blank"
                    rel="noreferrer"
                    className="underline"
                  >
                    manifest flow docs
                  </a>{' '}
                  for why the callback has to be a real, reachable address.
                </span>
              </p>
            ) : null}
          </div>

          {status.connected ? (
            <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
              <DialogTrigger
                render={<Button variant="destructive" size="sm" />}
              >
                Disconnect
              </DialogTrigger>
              <DialogContent className="sm:max-w-sm">
                <DialogHeader>
                  <DialogTitle className="flex items-center gap-1.5 text-destructive">
                    <WarningIcon className="size-4" aria-hidden="true" />
                    Disconnect GitHub App?
                  </DialogTitle>
                  <DialogDescription>
                    This stops this control plane from using the App to list
                    repositories or branches. It does not delete or uninstall
                    the App on GitHub itself; remove it from
                    github.com/settings/apps if you want it gone entirely.
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
                          toast.add({
                            title: 'GitHub App disconnected.',
                            type: 'success',
                          })
                        },
                        onError: (error) => {
                          toast.add({
                            title: 'Could not disconnect the GitHub App.',
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
          ) : (
            <div className="flex shrink-0 items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setManualOpen(true)}
              >
                Connect manually
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={!hasPrimaryDomain}
                onClick={() => {
                  window.location.href = '/api/v1/github-app/register/start'
                }}
              >
                <GithubLogoIcon className="size-4" />
                Add GitHub App
              </Button>
            </div>
          )}
        </div>
      </CardContent>
      <ManualConnectDialog open={manualOpen} onOpenChange={setManualOpen} />
    </Card>
  )
}

// ManualConnectDialog is Coolify's "Manual installation" card, adapted
// to a dialog: an operator creates the App themselves at
// github.com/settings/apps/new (no manifest, no redirect back here) and
// pastes the resulting credentials in directly. Every field GitHub's
// own App "General" settings page shows. installation_id/account_login
// are optional: an App saved without them still connects (status.installed
// stays false), the same two-step shape the automated flow's own
// install-after-create step has, see handleConnectGitHubAppManually's
// own doc comment.
function ManualConnectDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const connect = useConnectGitHubAppManually()
  const [appID, setAppID] = useState('')
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [webhookSecret, setWebhookSecret] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [installationID, setInstallationID] = useState('')
  const [accountLogin, setAccountLogin] = useState('')

  function resetForm() {
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
        client_secret: clientSecret,
        webhook_secret: webhookSecret,
        private_key: privateKey,
        installation_id:
          installationIDNum !== undefined && Number.isFinite(installationIDNum)
            ? installationIDNum
            : undefined,
        account_login: accountLogin.trim() || undefined,
      },
      {
        onSuccess: () => {
          toast.add({ title: 'GitHub App connected.', type: 'success' })
          resetForm()
          onOpenChange(false)
        },
        onError: (error) => {
          toast.add({
            title: 'Could not connect the GitHub App.',
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
            </a>
            , then paste the resulting credentials here. No primary domain
            required to save these: only automated setup needs one.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
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
            {connect.isPending ? 'Connecting...' : 'Connect'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
