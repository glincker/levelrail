import { useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useDeleteGlobalEnvironment } from '../../queries/globalEnvironments'
import type { GlobalEnvironment } from '../../types/environment'

export function DeleteEnvironmentDialog({
  trigger,
  environment,
  others,
}: {
  trigger: ReactElement
  environment: GlobalEnvironment
  others: GlobalEnvironment[]
}) {
  const { t } = useTranslation('environments')
  const [open, setOpen] = useState(false)
  const [moveTo, setMoveTo] = useState('')
  const remove = useDeleteGlobalEnvironment()
  const inUse = environment.app_count + environment.database_count > 0
  const targets = others.filter((e) => e.id !== environment.id && !e.protected)
  const blockedByProtection = inUse && environment.protected
  const canDelete =
    !remove.isPending && !blockedByProtection && (!inUse || moveTo !== '')

  function submit() {
    remove.mutate(
      { id: environment.id, moveTo: inUse ? moveTo : undefined },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: t('delete.deleted', { name: environment.name }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('delete.failed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('delete.title', { name: environment.name })}
          </DialogTitle>
          <DialogDescription>{t('delete.description')}</DialogDescription>
        </DialogHeader>
        {inUse ? (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              {t('delete.inUse', {
                apps: environment.app_count,
                databases: environment.database_count,
              })}
            </p>
            {blockedByProtection ? (
              <p className="text-sm text-destructive">
                {t('delete.protectedBlock')}
              </p>
            ) : (
              <div className="space-y-1.5">
                <label
                  htmlFor="environment-move-to"
                  className="text-sm font-medium text-foreground"
                >
                  {t('delete.moveTo')}
                </label>
                <Select
                  value={moveTo}
                  onValueChange={(next) => {
                    setMoveTo(next ?? '')
                  }}
                >
                  <SelectTrigger id="environment-move-to" className="w-full">
                    <SelectValue placeholder={t('delete.moveToNone')} />
                  </SelectTrigger>
                  <SelectContent>
                    {targets.map((e) => (
                      <SelectItem key={e.id} value={e.id}>
                        {e.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            {t('delete.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={!canDelete}
            onClick={submit}
          >
            {remove.isPending ? t('delete.deleting') : t('delete.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
