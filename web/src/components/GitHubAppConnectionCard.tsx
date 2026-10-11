import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  GithubLogoIcon,
  ArrowSquareOutIcon,
  PlusIcon,
  WarningIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { RelativeTime } from '@/components/kit'
import { useIngressSettings } from '../queries/domains'
import {
  useDeleteGitHubAppInstallation,
  useDisconnectGitHubApp,
  useGitHubAppInstallations,
  useGitHubAppStatus,
} from '../queries/githubApp'
import { SetPrimaryDomainPrompt } from './SetPrimaryDomainPrompt'
import { GitHubAppManifestDialog } from './GitHubAppManifestDialog'
import { ManualConnectDialog } from './GitHubAppManualConnectDialog'
import {
  ConnectionCardHeader,
  DisconnectConnectionDialog,
  mutationToastCallbacks,
} from './ConnectionCard'
import type { GitHubAppInstallation, GitHubAppStatus } from '@/types/githubApp'

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
      <ConnectionCardHeader
        icon={GithubLogoIcon}
        title="GitHub App"
        description="Connect a GitHub App for private-repository access and installation-based repo browsing."
      />
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
            <DisconnectConnectionDialog
              open={confirmOpen}
              onOpenChange={setConfirmOpen}
              title="Disconnect GitHub App?"
              description={
                <>
                  This stops this control plane from using the App to list
                  repositories or branches. It does not delete or uninstall the
                  App on GitHub itself; remove it from github.com/settings/apps
                  if you want it gone entirely.
                </>
              }
              pending={disconnect.isPending}
              onConfirm={() => {
                disconnect.mutate(
                  undefined,
                  mutationToastCallbacks(
                    'GitHub App disconnected.',
                    'Could not disconnect the GitHub App.',
                    () => setConfirmOpen(false),
                  ),
                )
              }}
            />
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
        {status.connected ? <GitHubAppInstallationsSection /> : null}
      </CardContent>
      <ManualConnectDialog open={manualOpen} onOpenChange={setManualOpen} />
      <GitHubAppManifestDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
      />
    </Card>
  )
}

// GitHubAppInstallationsSection lists every connected account/org
// (migrations/0282 made the App installable on more than one), each
// with its own disconnect button, plus the link to install on another
// one. Separate from the single legacy "installed as <account>" status
// above: that status reads github_app_connections' own single-row
// columns (kept for anything still reading them), this reads the real
// one-to-many table.
function GitHubAppInstallationsSection() {
  const { t } = useTranslation('settings')
  const { data, isLoading, isError, error } = useGitHubAppInstallations(true)

  return (
    <div className="space-y-2 border-t border-border pt-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium text-foreground">
          Connected accounts
        </p>
        {data?.add_org_url ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            render={
              <a href={data.add_org_url} target="_blank" rel="noreferrer" />
            }
          >
            <PlusIcon className="size-4" />
            {t('githubAppOrg.addOrg')}
          </Button>
        ) : null}
      </div>
      {data?.app_public === false ? (
        <div className="space-y-2 rounded-md border border-border bg-muted/40 p-3">
          <p className="text-sm text-foreground">
            {t('githubAppOrg.privateTitle')}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('githubAppOrg.privateBody')}
          </p>
          {data.make_public_url ? (
            <Button
              type="button"
              size="sm"
              render={
                <a
                  href={data.make_public_url}
                  target="_blank"
                  rel="noreferrer"
                />
              }
            >
              {t('githubAppOrg.makePublic')}
            </Button>
          ) : null}
        </div>
      ) : null}
      {data?.add_org_url && data.app_public !== false ? (
        <p className="text-xs text-muted-foreground">
          {t('githubAppOrg.addOrgHelp')}
        </p>
      ) : null}
      {!isLoading && !isError && !data?.add_org_url ? (
        <p className="text-xs text-muted-foreground">
          {t('githubAppOrg.noSlug')}
        </p>
      ) : null}
      {isLoading ? (
        <div className="space-y-1.5" aria-hidden="true">
          <Skeleton className="h-11 w-full" />
        </div>
      ) : null}
      {isError ? (
        <p className="text-sm text-destructive">
          {error instanceof Error
            ? error.message
            : 'Could not load connected accounts.'}
        </p>
      ) : null}
      {!isLoading && !isError && data ? (
        data.installations.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No accounts connected yet.
          </p>
        ) : (
          <ul className="space-y-1.5">
            {data.installations.map((installation) => (
              <GitHubAppInstallationRow
                key={installation.id}
                installation={installation}
              />
            ))}
          </ul>
        )
      ) : null}
    </div>
  )
}

export function GitHubAppInstallationRow({
  installation,
}: Readonly<{ installation: GitHubAppInstallation }>) {
  const { t } = useTranslation('settings')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const remove = useDeleteGitHubAppInstallation()

  return (
    <li className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2">
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-foreground">
          {installation.account_login}
        </p>
        <p className="text-xs text-muted-foreground">
          {t(
            installation.account_type === 'user'
              ? 'githubApp.personal'
              : 'githubApp.organization',
          )}
          , {t('githubApp.connected')}{' '}
          <RelativeTime at={installation.connected_at} />
        </p>
        {installation.settings_url ? (
          <a
            href={installation.settings_url}
            target="_blank"
            rel="noreferrer"
            title={t('githubApp.manageAccessHint')}
            className="inline-flex items-center gap-1 text-xs underline"
          >
            {t('githubApp.manageAccess')}
            <ArrowSquareOutIcon className="size-3" aria-hidden="true" />
          </a>
        ) : null}
      </div>
      <DisconnectConnectionDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={`Disconnect ${installation.account_login}?`}
        description={
          <>
            This stops this control plane from browsing or deploying from
            repositories under {installation.account_login}. Refused while a git
            source still points at one of its repos.
          </>
        }
        pending={remove.isPending}
        onConfirm={() => {
          remove.mutate(
            installation.id,
            mutationToastCallbacks(
              `Disconnected ${installation.account_login}.`,
              `Could not disconnect ${installation.account_login}.`,
              () => setConfirmOpen(false),
            ),
          )
        }}
      />
    </li>
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
function InstallationStatusMessage({
  status,
}: Readonly<{ status: GitHubAppStatus }>) {
  const { t } = useTranslation('settings')
  if (status.installation_status === 'suspended') {
    return (
      <p className="text-sm text-muted-foreground">
        GitHub reports this installation as suspended. Builds and repo browsing
        will fail until an admin un-suspends it from the App&apos;s page on
        GitHub.
      </p>
    )
  }
  if (status.installation_status === 'not_found') {
    return (
      <p className="text-sm text-muted-foreground">
        GitHub no longer has this installation on record, most likely because it
        was uninstalled there. Disconnect and reinstall the App to restore
        private-repository access.
      </p>
    )
  }
  if (!status.installed) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('githubApp.notInstalled')}
      </p>
    )
  }
  return null
}
