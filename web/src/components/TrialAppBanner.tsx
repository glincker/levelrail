import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FlaskConicalGlassIcon,
  PinGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useKeepTrialApp } from '../queries/trialApp'
import type { AppDetail } from '../types/appDetail'
import { DeleteAppDialog } from './DeleteAppDialog'
import { KeepTrialChecklist } from './trial/KeepTrialChecklist'

type TrialApp = Pick<
  AppDetail,
  'name' | 'domains' | 'volumes' | 'project_id' | 'environment_id'
>

// Shown on every section of a one-click trial deployment
// (store.DesiredService.IsTrial). "Stop & delete" reuses DeleteAppDialog;
// "Keep this app" makes it permanent through POST /apps/{name}/keep.
export function TrialAppBanner({ app }: { app: TrialApp }) {
  const { t } = useTranslation('trialApp')
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [keepOpen, setKeepOpen] = useState(false)
  const keep = useKeepTrialApp()

  const confirmKeep = () => {
    keep.mutate(app.name, {
      onSuccess: (res) => {
        setKeepOpen(false)
        toast.add({
          title: t(res.already_kept ? 'toast.alreadyKept' : 'toast.kept', {
            name: app.name,
          }),
          type: 'success',
        })
      },
    })
  }

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-tone-accent-border bg-tone-accent-soft p-3 text-tone-accent sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-start gap-2">
        <FlaskConicalGlassIcon
          className="mt-0.5 size-5 shrink-0"
          aria-hidden="true"
        />
        <div className="text-sm">
          <p className="font-medium">{t('banner.title')}</p>
          <p className="text-foreground/80">{t('banner.body')}</p>
        </div>
      </div>
      <div className="flex shrink-0 gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            keep.reset()
            setKeepOpen(true)
          }}
        >
          <PinGlassIcon className="size-5" aria-hidden="true" />
          {t('banner.keep')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="destructive"
          onClick={() => setDeleteOpen(true)}
        >
          <TrashGlassIcon className="size-5" aria-hidden="true" />
          {t('banner.delete')}
        </Button>
      </div>
      <ConfirmDialog
        open={keepOpen}
        onOpenChange={setKeepOpen}
        title={t('dialog.title', { name: app.name })}
        description={t('dialog.description')}
        confirmLabel={t('dialog.confirm')}
        pendingLabel={t('dialog.keeping')}
        cancelLabel={t('dialog.cancel')}
        pending={keep.isPending}
        error={keep.error ? keep.error.message || t('toast.failed') : null}
        onConfirm={confirmKeep}
      >
        {keepOpen ? <KeepTrialChecklist app={app} /> : null}
      </ConfirmDialog>
      <DeleteAppDialog
        name={app.name}
        control={{
          open: deleteOpen,
          onOpenChange: setDeleteOpen,
          hideTrigger: true,
        }}
      />
    </div>
  )
}
