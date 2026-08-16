import { useState } from 'react'
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
// Requires a primary domain to be configured first
// (store.IngressSettings.PrimaryDomain, PUT /api/v1/settings/ingress):
// GitHub needs a real, reachable https callback URL, and there is no
// meaningful fallback for "not configured yet" the backend could
// substitute (see internal/api's githubAppBaseURL doc comment). This
// card checks that client-side too, so the operator sees why the button
// is disabled instead of clicking through into a 409 on GitHub's own
// domain.
export function GitHubAppConnectionCard() {
  const { data: status } = useGitHubAppStatus()
  const { data: ingressSettings } = useIngressSettings()
  const disconnect = useDisconnectGitHubApp()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const hasPrimaryDomain = Boolean(ingressSettings.primary_domain)

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
                <a href="/settings/general" className="underline">
                  ingress settings
                </a>{' '}
                first: GitHub needs a real, reachable callback URL.
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
          )}
        </div>
      </CardContent>
    </Card>
  )
}
