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
import { Skeleton } from '@/components/ui/skeleton'
import { useIngressSettings } from '../queries/domains'
import {
  useConnectGitHubAppManually,
  useDisconnectGitHubApp,
  useGitHubAppManifestPreview,
  useGitHubAppStatus,
} from '../queries/githubApp'
import { SetPrimaryDomainPrompt } from './SetPrimaryDomainPrompt'
import type { GitHubAppStatus } from '@/types/githubApp'

// Status card for the GitHub App connection: not connected / connected
// as <account> but not yet installed / connected and installed on
// <account>. Two connect paths, side by side (Automated vs Manual),
// matching Coolify's own GitHub App screen: automated needs a real,
// publicly reachable primary domain (GitHub's servers redirect there
// for the App's whole registration lifetime); manual lets an operator
// paste credentials for an App they created themselves, no domain
// required.
export function GitHubAppConnectionCard() {
  const { data: status } = useGitHubAppStatus()
  const { data: ingressSettings } = useIngressSettings()
  const disconnect = useDisconnectGitHubApp()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [manualOpen, setManualOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)

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
                <InstallationBadge status={status} />
              </div>
            ) : null}
            {status.connected ? (
              <p className="font-mono text-sm text-muted-foreground">
                {status.instance_url}
              </p>
            ) : (
              <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
                <XCircleIcon className="size-4" />
                Not connected
              </div>
            )}
            {status.connected ? (
              <InstallationStatusMessage status={status} />
            ) : null}
            {!status.connected && !hasPrimaryDomain ? (
              <SetPrimaryDomainPrompt />
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
                  setPreviewOpen(true)
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
      <ManifestPreviewDialog open={previewOpen} onOpenChange={setPreviewOpen} />
    </Card>
  )
}

// InstallationBadge shows a distinct badge for each of the four states
// handleGetGitHubAppStatus can report: active, suspended (still
// installed on GitHub but usable by no one), not_found (uninstalled or
// deleted on GitHub's side), and never-installed. Suspended and
// not_found look different from each other and from "not installed yet"
// on purpose: they mean an operator has to go fix something on GitHub,
// not just finish a setup step still in progress.
function InstallationBadge({ status }: Readonly<{ status: GitHubAppStatus }>) {
  if (status.installation_status === 'suspended') {
    return <Badge variant="destructive">suspended on GitHub</Badge>
  }
  if (status.installation_status === 'not_found') {
    return <Badge variant="destructive">installation not found</Badge>
  }
  if (status.installed && status.account_login) {
    return <Badge variant="success">as {status.account_login}</Badge>
  }
  return <Badge variant="warning">not installed yet</Badge>
}

// InstallationStatusMessage is InstallationBadge's explanatory sibling:
// the badge alone doesn't say what to do about a suspended or missing
// installation.
function InstallationStatusMessage({ status }: Readonly<{ status: GitHubAppStatus }>) {
  if (status.installation_status === 'suspended') {
    return (
      <p className="text-sm text-muted-foreground">
        GitHub reports this installation as suspended. Builds and repo
        browsing will fail until an admin un-suspends it from the App&apos;s
        page on GitHub.
      </p>
    )
  }
  if (status.installation_status === 'not_found') {
    return (
      <p className="text-sm text-muted-foreground">
        GitHub no longer has this installation on record, most likely
        because it was uninstalled there. Disconnect and reinstall the App
        to restore private-repository access.
      </p>
    )
  }
  if (!status.installed) {
    return (
      <p className="text-sm text-muted-foreground">
        The App was created but hasn&apos;t been installed on an account
        or organization yet. Finish installing it on GitHub to pick
        repositories.
      </p>
    )
  }
  return null
}

// ManifestPreviewDialog shows exactly what handleStartGitHubAppRegistration
// is about to send GitHub before the browser navigates away: GitHub's
// own manifest confirmation page only lets an operator edit the App
// name, nothing else (callback/webhook/permissions must already be
// correct), so this is the one real chance to review them first. Fetches
// GET /api/v1/github-app/register/preview only while open (read-only,
// mints no CSRF state), and confirming does the same full-page
// navigation the old direct button click used to, just with an
// operator-editable name carried along as ?name=.
function ManifestPreviewDialog({
  open,
  onOpenChange,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const [instanceURL, setInstanceURL] = useState('')
  const trimmedInstanceURL = instanceURL.trim()
  const { data: preview, isLoading, isError, error } = useGitHubAppManifestPreview(
    trimmedInstanceURL,
    open,
  )
  const [name, setName] = useState('')
  // Empty means "untouched": falls back to the fetched default rather
  // than syncing it into state via an effect (React's own guidance
  // against setState-in-effect for derived initial values). Also
  // matches handleConfirm's own "blank name param means let the backend
  // pick" behavior below.
  const displayName = name === '' ? (preview?.app_name ?? '') : name

  function handleConfirm() {
    const params = new URLSearchParams()
    if (name.trim() !== '') {
      params.set('name', name.trim())
    }
    if (trimmedInstanceURL !== '') {
      params.set('instance_url', trimmedInstanceURL)
    }
    window.location.href = `/api/v1/github-app/register/start?${params.toString()}`
  }

  function renderPreviewBody() {
    if (isLoading) {
      return (
        <div className="space-y-4" aria-hidden="true">
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-24" />
            <Skeleton className="h-9 w-full" />
          </div>
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-16" />
            <Skeleton className="h-9 w-full" />
          </div>
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-full" />
            <Skeleton className="h-3.5 w-5/6" />
            <Skeleton className="h-3.5 w-2/3" />
          </div>
        </div>
      )
    }
    if (isError) {
      return (
        <p className="text-sm text-destructive">
          {error instanceof Error ? error.message : 'Could not load a preview.'}
        </p>
      )
    }
    if (!preview) {
      return null
    }
    return (
      <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
        <Field>
          <FieldLabel htmlFor="gh-preview-instance-url">
            GitHub instance
          </FieldLabel>
          <Input
            id="gh-preview-instance-url"
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
            Leave blank for github.com. Self-hosted GitHub Enterprise
            Server instances work too, e.g. https://github.example.com.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="gh-preview-name">App name</FieldLabel>
          <Input
            id="gh-preview-name"
            value={displayName}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
          <FieldDescription>
            The one field GitHub itself lets you change on its own
            confirmation page too.
          </FieldDescription>
        </Field>
        <div className="space-y-1.5 text-sm">
          <UrlRow label="Homepage" value={preview.homepage_url} />
          <UrlRow label="Callback" value={preview.callback_url} />
          <UrlRow label="Setup" value={preview.setup_url} />
          <UrlRow
            label="Webhook"
            value={preview.webhook_url}
            note={preview.webhook_active ? undefined : 'declared, not yet active'}
          />
        </div>
        <div className="space-y-1.5">
          <p className="text-sm font-medium text-foreground">Permissions requested</p>
          <ul className="space-y-1 text-sm text-muted-foreground">
            {Object.entries(preview.permissions).map(([key, level]) => (
              <li key={key} className="font-mono text-xs">
                {key}: {level}
              </li>
            ))}
          </ul>
        </div>
      </div>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setName('')
          setInstanceURL('')
        }
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Review before connecting to GitHub</DialogTitle>
          <DialogDescription>
            GitHub&apos;s own confirmation page only lets you rename the App;
            everything else below is fixed before your browser leaves this
            page.
          </DialogDescription>
        </DialogHeader>
        {renderPreviewBody()}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" disabled={!preview} onClick={handleConfirm}>
            Continue to GitHub
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function UrlRow({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate text-right font-mono text-xs">
        {value}
        {note ? <span className="ml-1.5 text-amber-700 dark:text-amber-400">({note})</span> : null}
      </span>
    </div>
  )
}

// ManualConnectDialog: an operator creates the App themselves at
// github.com/settings/apps/new and pastes the resulting credentials
// in directly. installation_id/account_login are optional: an App
// saved without them still connects (status.installed stays false).
function ManualConnectDialog({
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
            </a>{' '}
            (or your GitHub Enterprise Server instance&apos;s own
            /settings/apps/new), then paste the resulting credentials here.
            No primary domain required to save these: only automated setup
            needs one.
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
              Leave blank for github.com. Self-hosted instances work too,
              e.g. https://github.example.com.
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
