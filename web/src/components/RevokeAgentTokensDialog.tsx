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
import { useRevokeAgentTokens } from '../queries/aiControl'

export function RevokeAgentTokensDialog({ disabled }: { disabled: boolean }) {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const revoke = useRevokeAgentTokens()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) revoke.reset()
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button variant="outline" size="sm" disabled={disabled} />}
      >
        {t('aiControl.revoke.button')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WarningIcon className="size-4 text-destructive" />
            {t('aiControl.revoke.title')}
          </DialogTitle>
          <DialogDescription>
            {t('aiControl.revoke.description')}
          </DialogDescription>
        </DialogHeader>
        {revoke.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{revoke.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => handleOpenChange(false)}
          >
            {t('aiControl.revoke.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() =>
              revoke.mutate(undefined, {
                onSuccess: (r) => {
                  setOpen(false)
                  toast.add({
                    title: t('aiControl.revoke.done', { count: r.revoked }),
                    type: 'success',
                  })
                },
              })
            }
          >
            {revoke.isPending
              ? t('aiControl.revoke.revoking')
              : t('aiControl.revoke.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
