import { useState } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import {
  CheckIcon,
  CopyIcon,
  KeyIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  deviceAuthRequestsQueryOptions,
  useDeviceAuthRequests,
} from '../../queries/deviceAuth'
import { DeviceAuthRequestTable } from '../../components/DeviceAuthRequestTable'
import { Button } from '@/components/ui/button'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageHeader } from '@/components/shell/PageHeader'
import { HelpLink } from '../../components/HelpLink'

// Matches the copy-row idiom GitSourceWebhookBanner already established
// (code block + copy button in a muted bordered row); kept local since
// this page is the only place showing a shell command rather than a
// webhook URL/secret.
function CopyCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
      <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
        {command}
      </code>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          void navigator.clipboard.writeText(command).then(() => {
            setCopied(true)
            setTimeout(() => {
              setCopied(false)
            }, 2000)
          })
        }}
      >
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <CopyIcon aria-hidden="true" />
        )}
        {copied ? 'Copied' : 'Copy'}
      </Button>
    </div>
  )
}

interface CliAccessSearch {
  user_code?: string
}

// A plain function, not zod, matching reset-password.tsx's own reasoning:
// validateSearch runs as part of eager route matching, so pulling zod in
// here for one optional string would undo autoCodeSplitting's work.
function validateCliAccessSearch(
  search: Record<string, unknown>,
): CliAccessSearch {
  const { user_code } = search
  return typeof user_code === 'string' ? { user_code } : {}
}

// Web half of "levelrail-cli auth login --device": the CLI prints a
// user_code and polls POST /api/v1/auth/device/token, this page is
// where an operator sees that same code and approves or denies it.
// device_auth.go's VerificationURIComplete already points the CLI's
// printed link at /settings/cli-access?user_code=..., which
// validateSearch reads to highlight the matching row.
export const Route = createFileRoute('/settings/cli-access')({
  validateSearch: validateCliAccessSearch,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(deviceAuthRequestsQueryOptions()),
  component: CliAccessPage,
  pendingComponent: CliAccessPending,
})

function CliAccessPage() {
  const { data: requests } = useDeviceAuthRequests()
  const { user_code: highlightUserCode } = Route.useSearch()
  // Same origin the dashboard itself is served from: in production the
  // API and web UI are one embedded binary (CLAUDE.md 4.12), so this is
  // exactly the address an operator's CLI needs, no separate lookup.
  const apiUrl = window.location.origin

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <TerminalWindowIcon className="size-4" />
        </div>
        <PageHeader
          title="CLI access"
          description="Control this instance from your own computer, like aws or gh, not over SSH."
        />
      </div>

      <div className="space-y-3 rounded-lg border border-border p-4">
        <div className="flex items-center justify-between gap-4">
          <h3 className="text-sm font-medium">
            1. Install levelrail-cli on your computer
          </h3>
          <HelpLink
            path="/cli-reference"
            label="Full CLI reference"
            variant="inline"
          />
        </div>
        <p className="text-sm text-muted-foreground">
          Not the server. This is a small client binary for your laptop or CI
          runner, separate from the control plane install.
        </p>
        <CopyCommand command="curl -fsSL https://levelrail.com/install-cli.sh | sh" />
        <p className="text-xs text-muted-foreground">
          No root needed, installs to ~/.local/bin. macOS and Linux only today;
          on Windows, run this from WSL.
        </p>

        <h3 className="pt-2 text-sm font-medium">2. Point it at this server</h3>
        <CopyCommand command={`export APP_API_URL=${apiUrl}`} />
        <p className="text-sm text-muted-foreground">
          Set this once per shell (or in your shell profile); every command
          below talks to that address. Defaults to localhost otherwise.
        </p>

        <h3 className="pt-2 text-sm font-medium">3. Log in</h3>
        <CopyCommand command="levelrail-cli auth login --device" />
        <p className="text-sm text-muted-foreground">
          Opens a browser code here to approve, the same flow{' '}
          <code className="text-xs">gh auth login</code> and{' '}
          <code className="text-xs">aws sso login</code> use: no SSH key, no
          server port to open. Approving requires an already-signed-in dashboard
          session, so it inherits whatever 2FA or passkey that session already
          required, nothing extra to configure here.
        </p>
      </div>

      <div className="flex items-start gap-3 rounded-lg border border-border bg-muted/30 p-4 text-sm">
        <KeyIcon
          aria-hidden="true"
          className="mt-0.5 size-4 shrink-0 text-muted-foreground"
        />
        <p className="text-muted-foreground">
          Running from CI or a script instead of a person at a keyboard? Use an{' '}
          <Link
            to="/settings/tokens"
            className="font-medium text-foreground underline underline-offset-4 hover:no-underline"
          >
            API token
          </Link>{' '}
          instead, nothing to approve by hand.
        </p>
      </div>

      <DeviceAuthRequestTable
        requests={requests}
        highlightUserCode={highlightUserCode}
      />
    </div>
  )
}

function CliAccessPending() {
  return (
    <div className="space-y-6">
      <PageHeader title="CLI access" />
      <TableSkeleton columnCount={5} />
    </div>
  )
}
