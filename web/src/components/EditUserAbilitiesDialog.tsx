import { useState } from 'react'
import { PencilSimpleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { AbilitiesField } from './AbilitiesField'
import { RoleSelect } from './RoleSelect'
import { toast } from './ui/toast'
import { useUpdateUserAbilities } from '../queries/users'
import type { UserResource } from '../queries/users'
import { useRoles, roleForAbilities } from '../queries/roles'
import type { Ability } from '../types/token'

// PUT /api/v1/users/{id}/abilities: AbilityRoot-gated, and refuses
// id === the caller's own user (handleUpdateUserAbilities's own doc
// comment, the self-lockout guard: a root user can never edit their own
// abilities, only another root user can). UserTable is responsible for
// never rendering this dialog's trigger on the caller's own row in the
// first place, so the restriction is visible in the UI, not just
// enforced server-side after a wasted round trip.
export function EditUserAbilitiesDialog({ user }: { user: UserResource }) {
  const [open, setOpen] = useState(false)
  const [abilities, setAbilities] = useState<Ability[]>(user.abilities)
  const updateAbilities = useUpdateUserAbilities()
  const { data: roles } = useRoles()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setAbilities(user.abilities)
    } else {
      updateAbilities.reset()
    }
  }

  function handleSave() {
    const matchedRole = roleForAbilities(roles, abilities)
    updateAbilities.mutate(
      {
        id: user.id,
        ...(matchedRole ? { role: matchedRole.name } : { abilities }),
      },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: `${user.email}'s abilities updated.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <PencilSimpleIcon />
        Edit abilities
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit abilities for {user.email}</DialogTitle>
          <DialogDescription>
            What this account can do on this control plane.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <RoleSelect roles={roles} abilities={abilities} onChange={setAbilities} />
          <AbilitiesField value={abilities} onChange={setAbilities} />
        </div>

        {updateAbilities.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>
              {updateAbilities.error.message}
            </AlertDescription>
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
          <Button
            type="button"
            disabled={updateAbilities.isPending || abilities.length === 0}
            onClick={handleSave}
          >
            {updateAbilities.isPending ? 'Saving...' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
