import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/dialog'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'
import { toast } from './ui/toast'
import { useSetPasswordChangeRequired } from '../queries/users'
import type { UserResource } from '../queries/users'

/** RequirePasswordChangeDialog sets or clears a forced password change for one user. */
export function RequirePasswordChangeDialog({ user }: { user: UserResource }) {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const mutation = useSetPasswordChangeRequired()
  const requiring = !user.must_change_password

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      mutation.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        {requiring
          ? t('usersPage.passwordChange.require')
          : t('usersPage.passwordChange.clear')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyGlassIcon className="size-5" />
            {requiring
              ? t('usersPage.passwordChange.requireTitle', {
                  email: user.email,
                })
              : t('usersPage.passwordChange.clearTitle', { email: user.email })}
          </DialogTitle>
          <DialogDescription>
            {requiring
              ? t('usersPage.passwordChange.requireBody')
              : t('usersPage.passwordChange.clearBody')}
          </DialogDescription>
        </DialogHeader>
        {mutation.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription>{mutation.error.message}</AlertDescription>
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
            {t('usersPage.passwordChange.cancel')}
          </Button>
          <Button
            type="button"
            variant={requiring ? 'destructive' : 'default'}
            disabled={mutation.isPending}
            onClick={() => {
              mutation.mutate(
                { id: user.id, required: requiring },
                {
                  onSuccess: () => {
                    setOpen(false)
                    toast.add({
                      title: requiring
                        ? t('usersPage.passwordChange.required', {
                            email: user.email,
                          })
                        : t('usersPage.passwordChange.cleared', {
                            email: user.email,
                          }),
                      type: 'success',
                    })
                  },
                },
              )
            }}
          >
            {mutation.isPending
              ? t('usersPage.passwordChange.saving')
              : requiring
                ? t('usersPage.passwordChange.confirmRequire')
                : t('usersPage.passwordChange.confirmClear')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
