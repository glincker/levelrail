import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  IdentificationBadgeIcon,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { EnvironmentGrantsEditor } from './EnvironmentGrantsEditor'
import { useRoles } from '../../queries/roles'
import {
  fetchGrants,
  useAssignRole,
  useEnvironmentChoices,
} from '../../queries/userAccess'
import type { UserResource } from '../../queries/users'

export function UserRoleDialog({ user }: { user: UserResource }) {
  const { t } = useTranslation('access')
  const [open, setOpen] = useState(false)
  const [roleId, setRoleId] = useState(user.role_id ?? '')
  const [grants, setGrants] = useState<string[]>([])
  const { data: roles } = useRoles()
  const assign = useAssignRole()

  const selected = roles.find((r) => r.id === roleId)
  const granted = selected?.visibility === 'granted'
  const environments = useEnvironmentChoices(open && granted)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setRoleId(user.role_id ?? '')
      setGrants([])
      void fetchGrants(user.id)
        .then(setGrants)
        .catch(() => {
          setGrants([])
        })
    } else {
      assign.reset()
    }
  }

  function handleSave() {
    assign.mutate(
      {
        userId: user.id,
        roleId,
        ...(granted ? { environmentIds: grants } : {}),
      },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: t('userRole.toast', {
              email: user.email,
              role: selected?.name ?? '',
            }),
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <IdentificationBadgeIcon />
        {t('userRole.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('userRole.title', { email: user.email })}
          </DialogTitle>
          <DialogDescription>{t('userRole.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="user-role-select">
              {t('userRole.label')}
            </FieldLabel>
            <Select
              value={roleId}
              onValueChange={(next) => {
                setRoleId(next ?? '')
              }}
            >
              <SelectTrigger id="user-role-select" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {roles
                  .filter((r) => r.id !== undefined)
                  .map((r) => (
                    <SelectItem key={r.id} value={r.id ?? ''}>
                      {r.name}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
            <FieldDescription className="text-xs">
              {selected?.description}
            </FieldDescription>
          </Field>
          {granted ? (
            <EnvironmentGrantsEditor
              environments={environments.data ?? []}
              loading={environments.isLoading}
              value={grants}
              onChange={setGrants}
            />
          ) : null}
        </div>
        {assign.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{assign.error.message}</AlertDescription>
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
            {t('userRole.cancel')}
          </Button>
          <Button
            type="button"
            disabled={assign.isPending || roleId === ''}
            onClick={handleSave}
          >
            {assign.isPending ? t('userRole.saving') : t('userRole.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
