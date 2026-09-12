import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  CheckIcon,
  CopyIcon,
  EnvelopeIcon,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { useCreateInvite } from '../queries/invites'
import { useRoles } from '../queries/roles'
import { hasAbility } from '../types/token'
import type { Ability } from '../types/token'
import type { CreateInviteResponse } from '../queries/invites'

// AbilityWrite-gated (POST /api/v1/invites, internal/api/invites.go's
// handleCreateInvite doc comment), not AbilityRoot: any session holding
// 'write' can reach this dialog's action. An invite always applies a
// curated role rather than hand-picked abilities: the extra
// AbilitiesField checkbox grid earns its complexity for an admin
// creating an account directly (CreateUserDialog), not for a one-line
// "invite someone" action, so this intentionally offers less.
const inviteMemberSchema = z.object({
  email: z
    .string()
    .trim()
    .min(1, 'Email is required')
    .email('Enter a valid email'),
  role: z.enum(['admin', 'operator', 'viewer']),
})

type InviteMemberFormValues = z.infer<typeof inviteMemberSchema>

// onCreated lets the Users settings page remember this invite's link
// (InvitesTable's own "known only for invites created this page session"
// doc comment explains why: the server never returns a plaintext token
// twice), so the pending list's copy button works immediately for an
// invite created without navigating away first.
//
// callerAbilities is the signed-in session's own resolved abilities: the
// role picker below only ever offers a role whose abilities are a subset
// of callerAbilities, a UX nicety mirroring handleCreateInvite's own
// server-side privilege cap (invites.go), which is the actual security
// boundary either way.
export function InviteMemberDialog({
  callerAbilities,
  onCreated,
}: {
  callerAbilities: Ability[]
  onCreated?: (invite: CreateInviteResponse) => void
}) {
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<CreateInviteResponse | null>(null)
  const [copied, setCopied] = useState(false)
  const createInvite = useCreateInvite()
  const { data: roles } = useRoles()
  const grantableRoles = roles.filter((role) =>
    role.abilities.every((a) => hasAbility(callerAbilities, a)),
  )
  // Prefer 'operator' as the default, same as before this cap existed,
  // falling back to whatever the caller can actually grant.
  const defaultRole = (grantableRoles.find((r) => r.name === 'operator') ??
    grantableRoles[0])?.name as InviteMemberFormValues['role'] | undefined
  const { control, register, handleSubmit, formState, reset } =
    useForm<InviteMemberFormValues>({
      resolver: zodResolver(inviteMemberSchema),
      defaultValues: { email: '', role: defaultRole ?? 'viewer' },
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      setCopied(false)
      reset()
      createInvite.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    createInvite.mutate(
      { email: values.email.trim(), role: values.role },
      {
        onSuccess: (invite) => {
          setCreated(invite)
          onCreated?.(invite)
        },
      },
    )
  })

  function copyLink() {
    if (!created) {
      return
    }
    void navigator.clipboard.writeText(created.link).then(() => {
      setCopied(true)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>
        <EnvelopeIcon />
        Invite member
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Invite created</DialogTitle>
              <DialogDescription>
                Send this link to &ldquo;{created.email}&rdquo;, or let the
                email below reach them if this control plane has SMTP
                configured.
              </DialogDescription>
            </DialogHeader>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {created.link}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={copyLink}
              >
                {copied ? <CheckIcon /> : <CopyIcon />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
            <p className="text-sm text-muted-foreground">
              Expires {new Date(created.expires_at).toLocaleString()}. If no
              email capability is configured, this link is the only way they can
              accept.
            </p>
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
                <EnvelopeIcon className="size-4 text-muted-foreground" />
                Invite member
              </DialogTitle>
              <DialogDescription>
                Send an invite link for a new account with the role you choose
                below.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                void onSubmit(e)
              }}
              className="space-y-4"
            >
              <Field>
                <FieldLabel htmlFor="invite-email">Email</FieldLabel>
                <Input
                  id="invite-email"
                  type="email"
                  placeholder="e.g. teammate@example.com"
                  {...register('email')}
                />
                <FieldError errors={[formState.errors.email]} />
              </Field>

              <Controller
                control={control}
                name="role"
                render={({ field }) => {
                  const matched = grantableRoles.find(
                    (r) => r.name === field.value,
                  )
                  return (
                    <Field>
                      <FieldLabel htmlFor="invite-role">Role</FieldLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <SelectTrigger id="invite-role" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {grantableRoles.map((role) => (
                            <SelectItem key={role.name} value={role.name}>
                              {role.name.charAt(0).toUpperCase() +
                                role.name.slice(1)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FieldDescription className="text-xs">
                        {matched?.description}
                      </FieldDescription>
                    </Field>
                  )
                }}
              />

              {createInvite.isError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>
                    {createInvite.error.message}
                  </AlertDescription>
                </Alert>
              ) : null}

              <DialogFooter>
                <Button type="submit" disabled={createInvite.isPending}>
                  {createInvite.isPending ? 'Sending invite...' : 'Send invite'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
