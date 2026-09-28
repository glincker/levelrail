import { useState } from 'react'
import { KeyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { parseRecipients, recipientProblem } from '../lib/controlPlaneDr'
import {
  useAckControlPlaneEscrow,
  useBuildControlPlaneEscrow,
  type ControlPlaneDr,
  type ControlPlaneEscrowBundle,
} from '../queries/controlPlaneDr'

function saveTextFile(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

export function ControlPlaneDrEscrow({ status }: { status: ControlPlaneDr }) {
  const [open, setOpen] = useState(false)
  const [recipients, setRecipients] = useState('')
  const [upload, setUpload] = useState(false)
  const [bundle, setBundle] = useState<ControlPlaneEscrowBundle | null>(null)
  const build = useBuildControlPlaneEscrow()
  const ack = useAckControlPlaneEscrow()

  const override = parseRecipients(recipients)
  const problem = recipientProblem(override)
  const canGenerate = status.recipients.length > 0 || override.length > 0
  const close = () => {
    setOpen(false)
    setBundle(null)
    setRecipients('')
    setUpload(false)
    build.reset()
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-foreground">Key escrow</p>
          <p className="text-xs text-muted-foreground">
            The database is useless without the master key. Download it
            encrypted to your keys and store it offline.
            {status.escrow_acked_at !== undefined
              ? ' Stored and confirmed.'
              : status.escrow_generated_at !== undefined
                ? ' Generated, not yet confirmed as stored.'
                : ''}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setOpen(true)
          }}
        >
          <KeyIcon className="size-3.5" aria-hidden="true" />
          Escrow bundle
        </Button>
      </div>

      <Dialog
        open={open}
        onOpenChange={(o) => {
          if (!o) close()
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Master key escrow bundle</DialogTitle>
            <DialogDescription>
              The master key is encrypted on the server to your public keys.
              Only a holder of a matching private key can open the file.
            </DialogDescription>
          </DialogHeader>
          <div
            role="note"
            className="flex items-start gap-2 rounded-md border border-border bg-muted/40 p-3 text-sm text-muted-foreground"
          >
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            <span>
              Store the file offline, apart from your backups. If the backup and
              the escrow sit in the same bucket, anyone who reads that bucket
              gets both the data and its key.
            </span>
          </div>
          {bundle === null ? (
            <>
              <Field>
                <FieldLabel htmlFor="cpdr-escrow-recipients">
                  Encrypt to different keys (optional, one per line)
                </FieldLabel>
                <Textarea
                  id="cpdr-escrow-recipients"
                  className="font-mono text-xs"
                  placeholder="Leave empty to use the backup recipients"
                  value={recipients}
                  onChange={(e) => {
                    setRecipients(e.target.value)
                  }}
                />
                {problem !== null ? (
                  <p className="text-sm text-destructive">{problem}</p>
                ) : null}
              </Field>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={upload}
                  disabled={status.escrow_target_id === ''}
                  onCheckedChange={(c) => {
                    setUpload(c === true)
                  }}
                />
                Also upload to the escrow destination
                {status.escrow_target_id === '' ? ' (none configured)' : ''}
              </label>
              {build.isError ? (
                <p className="text-sm text-destructive">
                  {build.error.message}
                </p>
              ) : null}
              <DialogFooter>
                <Button variant="outline" onClick={close}>
                  Cancel
                </Button>
                <Button
                  disabled={build.isPending || problem !== null || !canGenerate}
                  onClick={() => {
                    build.mutate(
                      { recipients: override, upload },
                      {
                        onSuccess: (b) => {
                          saveTextFile(
                            `escrow-${b.created_at.slice(0, 10)}.age`,
                            b.armored,
                          )
                          saveTextFile(
                            `escrow-${b.created_at.slice(0, 10)}.instructions.txt`,
                            b.instructions,
                          )
                          setBundle(b)
                        },
                      },
                    )
                  }}
                >
                  {build.isPending ? 'Generating...' : 'Generate and download'}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <p className="text-sm text-foreground">
                Downloaded, encrypted to {bundle.recipient_count} recipient
                {bundle.recipient_count === 1 ? '' : 's'} (fingerprint{' '}
                <span className="font-mono">{bundle.fingerprint}</span>).
                {bundle.uploaded_key !== undefined
                  ? ` Also uploaded as ${bundle.uploaded_key}.`
                  : ''}
              </p>
              {ack.isError ? (
                <p className="text-sm text-destructive">{ack.error.message}</p>
              ) : null}
              <DialogFooter>
                <Button variant="outline" onClick={close}>
                  Not stored yet
                </Button>
                <Button
                  disabled={ack.isPending}
                  onClick={() => {
                    ack.mutate(undefined, {
                      onSuccess: () => {
                        toast.add({
                          title: 'Escrow confirmed as stored offline.',
                          type: 'success',
                        })
                        close()
                      },
                    })
                  }}
                >
                  I stored it offline
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
