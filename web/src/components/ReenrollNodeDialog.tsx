import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  CheckIcon,
  CopyIcon,
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
import {
  reenrollCommand,
  useCreateNodeReenrollToken,
} from '../queries/nodeCert'
import type { NodeReenrollTokenResponse } from '../types/nodeCert'
import type { NodeResource } from '../types/nodeDetail'

// Generates a re-enrollment token for an existing node and shows the
// command once, the same shape as AddNodeDialog.
export function ReenrollNodeDialog({ node }: { node: NodeResource }) {
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<NodeReenrollTokenResponse | null>(null)
  const [copied, setCopied] = useState(false)
  const [controlPlaneAddr, setControlPlaneAddr] = useState(
    () => `${window.location.hostname}:9443`,
  )
  const createToken = useCreateNodeReenrollToken()
  const command = created ? reenrollCommand(created, controlPlaneAddr) : ''

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      setCopied(false)
      createToken.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
        Re-enroll node
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Re-enroll &ldquo;{node.name}&rdquo;</DialogTitle>
          <DialogDescription>
            Issues this node a new agent certificate while keeping its identity,
            placements and history. Use it when the node was offline past its
            certificate&apos;s expiry, or after revoking its certificate.
          </DialogDescription>
        </DialogHeader>

        {created ? (
          <>
            <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
              <WarningIcon className="mt-0.5 size-4 shrink-0" />
              <p className="text-sm">
                This token will not be shown again. It works once and expires at{' '}
                {new Date(created.expires_at).toLocaleTimeString()}.
              </p>
            </div>
            <Field>
              <FieldLabel htmlFor="reenroll-control-plane-addr">
                Control plane address
              </FieldLabel>
              <Input
                id="reenroll-control-plane-addr"
                value={controlPlaneAddr}
                onChange={(e) => {
                  setControlPlaneAddr(e.target.value)
                  setCopied(false)
                }}
                className="font-mono text-xs"
              />
              <FieldDescription>
                Host and port the node reaches this control plane on, port 9443
                by default.
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="reenroll-command">
                Run on the node
              </FieldLabel>
              <div className="flex items-start gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code
                  id="reenroll-command"
                  className="min-w-0 flex-1 overflow-x-auto text-xs break-all whitespace-pre-wrap"
                >
                  {command}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(command).then(() => {
                      setCopied(true)
                    })
                  }}
                >
                  {copied ? <CheckIcon /> : <CopyIcon />}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              </div>
              <FieldDescription>
                A running agent picks up the new certificate at its next
                reconnect; otherwise start the agent after this command.
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
            {createToken.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>{createToken.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                disabled={createToken.isPending}
                onClick={() => {
                  createToken.mutate(node.id, { onSuccess: setCreated })
                }}
              >
                {createToken.isPending
                  ? 'Generating...'
                  : 'Generate re-enroll token'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
