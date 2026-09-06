import { useState } from 'react'
import {
  CheckCircleIcon,
  EyeIcon,
  EyeSlashIcon,
  KeyIcon,
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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useRotateMasterKey } from '../queries/masterKey'
import type { RotateMasterKeyResult } from '../queries/masterKey'

const CONFIRM_PHRASE = 'ROTATE'

// The single key every other secret this control plane stores depends
// on (see docs/master-key-rotation.md), so this gets the same
// type-to-confirm friction RestoreVolumeBackupDialog's own comment
// establishes for the most consequential actions in this dashboard,
// plus a new-key input: unlike a plain confirm dialog, the operator
// must supply the new key's own value (generated ahead of time, the
// same file-based flow "levelrail-cli secrets rotate-master-key
// --new-key-file" already uses), this dialog only wraps the same POST
// /api/v1/system/master-key/rotate call with a paste target instead of
// a file read.
//
// Unlike every other dialog in this codebase, a successful rotation
// does not auto-close: result.persistedToFile and result.warning are
// operationally critical (a false persistedToFile means the operator
// must update APP_MASTER_KEY themselves before the next restart, or
// every secret becomes unreadable), so they're shown as a panel the
// operator has to explicitly dismiss, not a toast that can be missed.
export function RotateMasterKeyDialog() {
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [newKey, setNewKey] = useState('')
  const [revealKey, setRevealKey] = useState(false)
  const [result, setResult] = useState<RotateMasterKeyResult | null>(null)
  const rotate = useRotateMasterKey()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setConfirmText('')
      setNewKey('')
      setRevealKey(false)
      setResult(null)
      rotate.reset()
    }
  }

  const confirmed = confirmText === CONFIRM_PHRASE && newKey.trim().length > 0

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <KeyIcon className="size-3.5" aria-hidden="true" />
        Rotate master key
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {result ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-1.5">
                <CheckCircleIcon className="size-4 text-green-600 dark:text-green-400" aria-hidden="true" />
                Master key rotated
              </DialogTitle>
              <DialogDescription>
                Rotated at {new Date(result.rotatedAt).toLocaleString()}. Every
                stored secret was re-wrapped under the new key; nothing else
                changed.
              </DialogDescription>
            </DialogHeader>
            {result.persistedToFile ? (
              <Alert>
                <CheckCircleIcon />
                <AlertTitle>Written to the master key file</AlertTitle>
                <AlertDescription>
                  This control plane will pick up the new key automatically on
                  its next restart. No action needed.
                </AlertDescription>
              </Alert>
            ) : (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertTitle>Not persisted to disk</AlertTitle>
                <AlertDescription>
                  {result.warning ??
                    'The new key was applied live but not written to a key file (env-sourced master key, or the file write failed). You must update APP_MASTER_KEY (or the key file) with the new value yourself before this control plane next restarts, or it will be unable to read any stored secret.'}
                </AlertDescription>
              </Alert>
            )}
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
              <DialogTitle className="flex items-center gap-1.5 text-destructive">
                <WarningIcon className="size-4" aria-hidden="true" />
                Rotate the master key?
              </DialogTitle>
              <DialogDescription>
                Re-wraps every stored secret (app env vars, SMTP credentials,
                git provider app secrets, backup target credentials, and
                more) under a new master key, live, without a restart. Keep
                the current key available until this succeeds: a rotation
                that fails partway through needs it to retry. See{' '}
                <a
                  href="https://github.com/glincker/levelrail/blob/main/docs/master-key-rotation.md"
                  target="_blank"
                  rel="noreferrer"
                  className="underline underline-offset-2"
                >
                  the rotation guide
                </a>{' '}
                before doing this for the first time.
              </DialogDescription>
            </DialogHeader>

            <Field>
              <FieldLabel htmlFor="master-key-new-value">
                New master key
              </FieldLabel>
              <div className="relative">
                <Input
                  id="master-key-new-value"
                  type={revealKey ? 'text' : 'password'}
                  className="pr-9 font-mono"
                  autoComplete="off"
                  spellCheck={false}
                  placeholder="AGE-SECRET-KEY-1..."
                  value={newKey}
                  onChange={(event) => {
                    setNewKey(event.target.value)
                  }}
                />
                <button
                  type="button"
                  onClick={() => {
                    setRevealKey((v) => !v)
                  }}
                  aria-label={revealKey ? 'Hide value' : 'Show value'}
                  aria-pressed={revealKey}
                  className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
                >
                  {revealKey ? (
                    <EyeSlashIcon className="size-4" />
                  ) : (
                    <EyeIcon className="size-4" />
                  )}
                </button>
              </div>
              <p className="text-xs text-muted-foreground">
                Generated ahead of time (the same value{' '}
                <code className="font-mono">
                  levelrail-cli secrets rotate-master-key --new-key-file
                </code>{' '}
                reads from a file). Never sent anywhere but this request.
              </p>
            </Field>

            <Field>
              <FieldLabel htmlFor="master-key-confirm-phrase">
                Type &ldquo;{CONFIRM_PHRASE}&rdquo; to confirm
              </FieldLabel>
              <Input
                id="master-key-confirm-phrase"
                autoComplete="off"
                spellCheck={false}
                value={confirmText}
                onChange={(event) => {
                  setConfirmText(event.target.value)
                }}
              />
            </Field>

            {rotate.isError ? (
              <p className="text-sm text-destructive">{rotate.error.message}</p>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                variant="destructive"
                disabled={!confirmed || rotate.isPending}
                onClick={() => {
                  rotate.mutate(newKey.trim(), {
                    onSuccess: (rotated) => {
                      setResult(rotated)
                    },
                  })
                }}
              >
                {rotate.isPending ? 'Rotating...' : 'Rotate now'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
