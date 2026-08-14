import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  PlusIcon,
  HardDrivesIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { useCreateNodeJoinToken } from '../queries/nodes'
import { useBrand } from '../hooks/useBrand'
import type { NodeJoinTokenResponse } from '../types/nodeDetail'

// A dialog, not a wizard: this product's own positioning (CLAUDE.md 1,
// "lightweight, AI-ready Coolify alternative") and both Coolify's and
// Dokploy's own "add server" flows (single screen, no multi-step
// wizard) back that call, and multi-node is an occasional admin action
// for the 1-10 machine target audience, not a daily-use flow. One
// dialog, two phases: explain + a "Generate join token" button, then
// (on success) the token plus the exact enrollment command, mirroring
// CreateTokenDialog's own "mint a secret, show it exactly once" shape.
export function AddNodeDialog() {
  const brand = useBrand()
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<NodeJoinTokenResponse | null>(null)
  const [tokenCopied, setTokenCopied] = useState(false)
  const [commandCopied, setCommandCopied] = useState(false)
  // Pre-filled once from the browser's own address bar (the honest
  // client-side default the task scope allows), then editable: the
  // dashboard's own hostname is usually the right reachable address for
  // the new node to dial, but not always (NAT, a separate public
  // address, etc.), and there is no config endpoint that could look
  // this up authoritatively. defaultAgentAddr (":9443",
  // cmd/levelrail/main.go) is the control plane's fixed agent gRPC
  // listener port, never the same port as this HTTP dashboard.
  const [controlPlaneAddr, setControlPlaneAddr] = useState(
    () => `${window.location.hostname}:9443`,
  )
  const createJoinToken = useCreateNodeJoinToken()

  // brand.BinaryName ("levelrail") names the control plane binary
  // (brand.yaml's binary_name); the agent binary follows the same
  // "<binary_name>-agent" convention cmd/levelrail vs cmd/levelrail-agent
  // already uses. Deriving it here instead of writing "levelrail-agent"
  // literally keeps this file honoring CLAUDE.md section 3: no product
  // name string hardcoded anywhere under /web.
  const agentBinaryName = `${brand.BinaryName}-agent`
  const enrollCommand = created
    ? `APP_CONTROL_PLANE_ADDR=${controlPlaneAddr} APP_JOIN_TOKEN=${created.token} ./${agentBinaryName}`
    : ''

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      setTokenCopied(false)
      setCommandCopied(false)
      createJoinToken.reset()
    }
  }

  function copyToken() {
    if (!created) {
      return
    }
    void navigator.clipboard.writeText(created.token).then(() => {
      setTokenCopied(true)
    })
  }

  function copyCommand() {
    void navigator.clipboard.writeText(enrollCommand).then(() => {
      setCommandCopied(true)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusIcon />
        Add node
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Join token created</DialogTitle>
              <DialogDescription>
                Run the command below on the new machine to enroll it. The token
                is a one-time bootstrap credential, not something to save
                long-term.
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
              <WarningIcon className="mt-0.5 size-4 shrink-0" />
              <p className="text-sm">
                This token will not be shown again. It expires at{' '}
                {formatTime(created.expires_at)} (valid for 15 minutes).
              </p>
            </div>

            <Field>
              <FieldLabel htmlFor="node-join-token">Join token</FieldLabel>
              <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code
                  id="node-join-token"
                  className="min-w-0 flex-1 overflow-x-auto text-xs break-all"
                >
                  {created.token}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={copyToken}
                >
                  {tokenCopied ? <CheckIcon /> : <CopyIcon />}
                  {tokenCopied ? 'Copied' : 'Copy'}
                </Button>
              </div>
            </Field>

            <Field>
              <FieldLabel htmlFor="node-control-plane-addr">
                Control plane address
              </FieldLabel>
              <Input
                id="node-control-plane-addr"
                value={controlPlaneAddr}
                onChange={(e) => {
                  setControlPlaneAddr(e.target.value)
                  setCommandCopied(false)
                }}
                className="font-mono text-xs"
              />
              <FieldDescription>
                Host and port the new machine can reach this control plane on,
                port 9443 by default. Pre-filled from this browser&apos;s own
                address, change it if the new machine reaches this control plane
                a different way (NAT, a separate public address).
              </FieldDescription>
            </Field>

            <Field>
              <FieldLabel htmlFor="node-enroll-command">
                Enrollment command
              </FieldLabel>
              <div className="flex items-start gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code
                  id="node-enroll-command"
                  className="min-w-0 flex-1 overflow-x-auto text-xs break-all whitespace-pre-wrap"
                >
                  {enrollCommand}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={copyCommand}
                >
                  {commandCopied ? <CheckIcon /> : <CopyIcon />}
                  {commandCopied ? 'Copied' : 'Copy'}
                </Button>
              </div>
              <FieldDescription>
                Run this on the new machine. There is no live status check here:
                refresh this page once the agent connects to see it appear in
                the node list.
              </FieldDescription>
            </Field>

            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <HardDrivesIcon className="size-4 text-muted-foreground" />
                Add node
              </DialogTitle>
              <DialogDescription>
                Generates a one-time join token, good for 15 minutes. Run the
                enrollment command it produces on the new machine to connect its
                agent to this control plane.
              </DialogDescription>
            </DialogHeader>

            {createJoinToken.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>
                  {createJoinToken.error.message}
                </AlertDescription>
              </Alert>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                disabled={createJoinToken.isPending}
                onClick={() => {
                  createJoinToken.mutate(undefined, { onSuccess: setCreated })
                }}
              >
                {createJoinToken.isPending
                  ? 'Generating...'
                  : 'Generate join token'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

// "Expires at HH:MM" per the task's own plain-notice option, not a
// countdown: a countdown implies urgency worth animating for a token
// whose whole window is a flat 15 minutes, plain wall-clock time reads
// faster.
function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
}
