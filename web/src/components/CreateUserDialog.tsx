import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { UserPlusIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { AbilitiesField } from './AbilitiesField'
import { RoleSelect } from './RoleSelect'
import { toast } from './ui/toast'
import { useCreateUser } from '../queries/users'
import { useRoles, roleForAbilities } from '../queries/roles'

// AbilityRoot-gated (POST /api/v1/auth/users, internal/api/users.go's
// handleCreateUser doc comment): the caller picks the new user's
// Abilities, so only a root user can reach this dialog's action, the
// same reasoning CreateTokenDialog's own abilities picker documents for
// tokens. Every route this dialog's trigger appears on already requires
// a root session to load the Users page's edit affordances in the first
// place, but the server enforces this regardless.
const createUserSchema = z.object({
  email: z.string().trim().min(1, 'Email is required'),
  displayName: z.string().trim(),
  password: z.string().min(8, 'Password must be at least 8 characters'),
  abilities: z
    .array(
      z.enum([
        'read',
        'read:sensitive',
        'write',
        'write:sensitive',
        'deploy',
        'root',
      ]),
    )
    .min(1, 'Select at least one ability'),
})

type CreateUserFormValues = z.infer<typeof createUserSchema>

export function CreateUserDialog() {
  const [open, setOpen] = useState(false)
  const createUser = useCreateUser()
  const { data: roles } = useRoles()
  const { control, register, handleSubmit, formState, reset } =
    useForm<CreateUserFormValues>({
      resolver: zodResolver(createUserSchema),
      defaultValues: { email: '', displayName: '', password: '', abilities: [] },
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset()
      createUser.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    // A curated role match sends `role` (server-resolved, roles.go),
    // otherwise the hand-picked `abilities` list: whichever the current
    // checkbox state actually represents, RoleSelect's own derivation.
    const matchedRole = roleForAbilities(roles, values.abilities)
    createUser.mutate(
      {
        email: values.email.trim(),
        ...(values.displayName.trim()
          ? { display_name: values.displayName.trim() }
          : {}),
        password: values.password,
        ...(matchedRole
          ? { role: matchedRole.name }
          : { abilities: values.abilities }),
      },
      {
        onSuccess: (user) => {
          handleOpenChange(false)
          toast.add({ title: `${user.email} created.`, type: 'success' })
        },
      },
    )
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>
        <UserPlusIcon />
        Create user
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UserPlusIcon className="size-4 text-muted-foreground" />
            Create user
          </DialogTitle>
          <DialogDescription>
            A local-password account with the abilities you choose below.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="new-user-email">Email</FieldLabel>
            <Input
              id="new-user-email"
              type="email"
              placeholder="e.g. teammate@example.com"
              {...register('email')}
            />
            <FieldError errors={[formState.errors.email]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="new-user-display-name">
              Display name
            </FieldLabel>
            <Input
              id="new-user-display-name"
              placeholder="Defaults to the email"
              {...register('displayName')}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="new-user-password">Password</FieldLabel>
            <Input
              id="new-user-password"
              type="password"
              {...register('password')}
            />
            <FieldError errors={[formState.errors.password]} />
          </Field>

          <Controller
            control={control}
            name="abilities"
            render={({ field }) => (
              <div className="space-y-4">
                <RoleSelect
                  roles={roles}
                  abilities={field.value}
                  onChange={field.onChange}
                />
                <AbilitiesField
                  value={field.value}
                  onChange={field.onChange}
                  error={formState.errors.abilities}
                />
              </div>
            )}
          />

          {createUser.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{createUser.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={createUser.isPending}>
              {createUser.isPending ? 'Creating...' : 'Create user'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
