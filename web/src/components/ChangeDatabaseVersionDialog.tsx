import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSetDatabaseVersion } from '../queries/databases'

// PUT /api/v1/databases/{name}/version: the API refuses a major change with
// a 409 whose message is shown inline.
export function ChangeDatabaseVersionDialog({
  name,
  currentVersion,
}: {
  name: string
  currentVersion: string
}) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const [version, setVersion] = useState(currentVersion)
  const setDatabaseVersion = useSetDatabaseVersion()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setVersion(currentVersion)
    } else {
      setDatabaseVersion.reset()
    }
  }

  const unchanged = version.trim() === currentVersion

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="xs" />}>
        <PencilSimpleIcon className="size-3" aria-hidden="true" />
        {t('version.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('version.title', { name })}</DialogTitle>
          <DialogDescription>{t('version.description')}</DialogDescription>
        </DialogHeader>
        <label className="space-y-1 text-sm font-medium">
          {t('version.label')}
          <Input
            value={version}
            placeholder={t('version.placeholder')}
            onChange={(e) => {
              setVersion(e.target.value)
            }}
            className="font-mono"
          />
        </label>
        {setDatabaseVersion.isError ? (
          <p className="text-sm text-destructive" role="alert">
            {setDatabaseVersion.error.message}
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
            {t('version.cancel')}
          </Button>
          <Button
            type="button"
            disabled={setDatabaseVersion.isPending || unchanged}
            onClick={() => {
              const next = version.trim()
              setDatabaseVersion.mutate(
                { name, version: next },
                {
                  onSuccess: () => {
                    setOpen(false)
                    toast.add({
                      title: t('version.toast', { name, version: next }),
                      type: 'success',
                    })
                  },
                },
              )
            }}
          >
            {setDatabaseVersion.isPending
              ? t('version.submitting')
              : t('version.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
