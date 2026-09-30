import { type FormEvent, useState } from 'react'
import {
  CheckCircleIcon,
  FingerprintIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  useDeletePasskey,
  usePasskeys,
  useRegisterPasskey,
} from '../queries/passkeys'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Passkeys card: lists the account's registered WebAuthn credentials and
// lets the operator add or revoke one. Follows TwoFactorCard's own
// visual and interaction conventions (settings/security.tsx) closely,
// split into its own file since that route is already at this
// project's 500-line-per-file ceiling.
export function PasskeysCard() {
  const { data: passkeys } = usePasskeys()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FingerprintIcon className="size-4 text-muted-foreground" />
          Passkeys
        </CardTitle>
        <CardDescription>
          Sign in with Touch ID, Windows Hello, or a security key instead of a
          password.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {passkeys.length > 0 ? (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {passkeys.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between gap-3 p-3 text-sm"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium text-foreground">
                    {p.label}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Added {formatDate(p.created_at)}
                    {p.last_used_at
                      ? `, last used ${formatDate(p.last_used_at)}`
                      : ', never used'}
                  </p>
                </div>
                <RemovePasskeyButton id={p.id} label={p.label} />
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            No passkeys registered yet.
          </p>
        )}
        <AddPasskeyDialog />
      </CardContent>
    </Card>
  )
}

function AddPasskeyDialog() {
  const [open, setOpen] = useState(false)
  const [label, setLabel] = useState('')
  const register = useRegisterPasskey()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setLabel('')
    } else {
      register.reset()
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    register.mutate(label.trim() || 'Passkey', {
      onSuccess: () => {
        handleOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" />}>
        <FingerprintIcon />
        Add a passkey
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <form onSubmit={handleSubmit} className="space-y-3">
          <DialogHeader>
            <DialogTitle>Add a passkey</DialogTitle>
            <DialogDescription>
              Give it a name you'll recognize, then follow your browser's prompt
              to use Touch ID, Windows Hello, or a security key.
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor="passkey-label">Name</FieldLabel>
            <Input
              id="passkey-label"
              value={label}
              onChange={(e) => {
                setLabel(e.target.value)
              }}
              placeholder="MacBook Touch ID"
              autoFocus
            />
          </Field>
          {register.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{register.error.message}</AlertDescription>
            </Alert>
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
            <Button type="submit" disabled={register.isPending}>
              {register.isPending ? (
                'Waiting for passkey...'
              ) : (
                <>
                  <CheckCircleIcon />
                  Continue
                </>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function RemovePasskeyButton({ id, label }: { id: string; label: string }) {
  const [open, setOpen] = useState(false)
  const del = useDeletePasskey()

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          del.reset()
        }
      }}
    >
      <DialogTrigger render={<Button variant="ghost" size="sm" />}>
        <TrashIcon />
        <span className="sr-only">Remove {label}</span>
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WarningIcon className="size-4 text-destructive" />
            Remove this passkey?
          </DialogTitle>
          <DialogDescription>
            {`"${label}" will no longer be able to sign in to this account.`}
          </DialogDescription>
        </DialogHeader>
        {del.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{del.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={del.isPending}
            onClick={() => {
              del.mutate(id, { onSuccess: () => setOpen(false) })
            }}
          >
            {del.isPending ? 'Removing...' : 'Remove'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
