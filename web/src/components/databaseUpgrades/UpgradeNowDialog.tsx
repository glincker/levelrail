import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowCircleUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useUpgradeNow } from '../../queries/databaseUpgrades'

/** UpgradeNowDialog starts a manual upgrade once the operator types the database name. */
export function UpgradeNowDialog({
  databaseName,
  version,
  disabled,
}: {
  databaseName: string
  version: string
  disabled?: boolean
}) {
  const { t } = useTranslation('databaseUpgrades')
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const upgrade = useUpgradeNow(databaseName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setTyped('')
      upgrade.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button variant="outline" size="sm" disabled={disabled} />}
      >
        <ArrowCircleUpIcon className="size-3.5" aria-hidden="true" />
        {t('targets.upgradeNow')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>
            {t('dialog.title', { name: databaseName, version })}
          </DialogTitle>
          <DialogDescription>{t('dialog.description')}</DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor={`upgrade-confirm-${version}`}>
            {t('dialog.confirmLabel', { name: databaseName })}
          </FieldLabel>
          <Input
            id={`upgrade-confirm-${version}`}
            value={typed}
            autoComplete="off"
            onChange={(e) => {
              setTyped(e.target.value)
            }}
          />
        </Field>
        {upgrade.isError ? (
          <p className="text-sm text-destructive" role="alert">
            {upgrade.error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('dialog.cancel')}
          </Button>
          <Button
            type="button"
            disabled={typed !== databaseName || upgrade.isPending}
            onClick={() => {
              upgrade.mutate(
                { version, confirm: typed },
                {
                  onSuccess: () => {
                    handleOpenChange(false)
                    toast.add({
                      title: t('dialog.toast', { version }),
                      type: 'success',
                    })
                  },
                  onError: (err) => {
                    toast.add({
                      title: t('dialog.failedToast'),
                      description: err.message,
                      type: 'error',
                    })
                  },
                },
              )
            }}
          >
            {upgrade.isPending ? t('dialog.starting') : t('dialog.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
