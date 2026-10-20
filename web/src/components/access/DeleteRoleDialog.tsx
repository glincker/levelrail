import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { toast } from '@/components/ui/toast'
import { useDeleteRole } from '../../queries/roles'
import type { RoleResource } from '../../queries/roles'

export function DeleteRoleDialog({ role }: { role: RoleResource }) {
  const { t } = useTranslation('access')
  const [open, setOpen] = useState(false)
  const del = useDeleteRole()
  const inUse = (role.user_count ?? 0) > 0

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
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        {t('roles.table.delete')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WarningIcon className="size-4 text-destructive" />
            {t('roles.deleteDialog.title', { name: role.name })}
          </DialogTitle>
          <DialogDescription>
            {t('roles.deleteDialog.description')}
          </DialogDescription>
        </DialogHeader>
        {inUse ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>
              {t('roles.deleteDialog.inUse', { count: role.user_count })}
            </AlertDescription>
          </Alert>
        ) : null}
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
            {t('roles.deleteDialog.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={del.isPending || inUse || !role.id}
            onClick={() => {
              if (!role.id) {
                return
              }
              del.mutate(role.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: t('roles.toast.deleted', { name: role.name }),
                    type: 'success',
                  })
                },
              })
            }}
          >
            {del.isPending
              ? t('roles.deleteDialog.deleting')
              : t('roles.deleteDialog.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
