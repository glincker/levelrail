import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useRollbackToDeploy } from '../../queries/deployControl'
import { useRollbackTargets } from '../../queries/deployGate'
import {
  useClearActionParam,
  type AppPaletteAction,
} from '../../lib/paletteAction'
import type { DeployTab } from '../overview/DeploySheet'
import type { AppDetail } from '../../types/appDetail'
import { useAppPower } from './useAppPower'

type ConfirmAction = 'rollback'

/** Opens the page's own confirm step for a ?action= link from the command palette. */
export function AppPaletteActions({
  app,
  action,
  onDeploy,
}: {
  app: AppDetail
  action: AppPaletteAction | undefined
  onDeploy: (tab: DeployTab) => void
}) {
  const { t } = useTranslation('common')
  const clear = useClearActionParam()
  const [active, setActive] = useState<ConfirmAction | null>(null)
  const power = useAppPower(app)
  const rollbackTo = useRollbackToDeploy(app.name)
  const { data: targets } = useRollbackTargets(app.name)
  const previous = targets?.find((target) => !target.live)

  const [handled, setHandled] = useState<AppPaletteAction>()
  if (action !== handled) {
    setHandled(action)
    if (
      action &&
      action !== 'deploy' &&
      !(action === 'stop' && app.suspended)
    ) {
      if (action === 'rollback') setActive(action)
      else power.ask(action)
    }
  }

  useEffect(() => {
    if (!action) return
    if (action === 'deploy') {
      onDeploy('existing-image')
    } else if (action === 'stop' && app.suspended) {
      toast.add({
        title: t('paletteAction.alreadyStopped', { name: app.name }),
        type: 'info',
      })
    }
    clear()
  }, [action, app.name, app.suspended, clear, onDeploy, t])

  const noPrevious = active === 'rollback' && Boolean(targets) && !previous
  useEffect(() => {
    if (noPrevious) {
      toast.add({
        title: t('paletteAction.rollbackNone', { name: app.name }),
        type: 'info',
      })
    }
  }, [noPrevious, app.name, t])

  const close = () => {
    setActive(null)
  }

  return (
    <>
      {power.confirmDialog}
      <ConfirmDialog
        open={active === 'rollback' && !noPrevious}
        onOpenChange={(open) => !open && close()}
        title={t('paletteAction.rollbackTitle', { name: app.name })}
        description={t('paletteAction.rollbackDescription', {
          image: previous?.image ?? '',
        })}
        consequences={[t('paletteAction.rollbackConsequence')]}
        confirmLabel={t('paletteAction.rollbackConfirm')}
        pendingLabel={t('paletteAction.pending')}
        tone="destructive"
        pending={rollbackTo.isPending}
        onConfirm={() => {
          if (!previous) return
          rollbackTo.mutate(
            { deployId: previous.deploy_id },
            {
              onSuccess: () => {
                toast.add({
                  title: t('paletteAction.rollbackStarted', {
                    name: app.name,
                  }),
                  type: 'success',
                })
                close()
              },
              onError: (error) =>
                toast.add({
                  title: t('paletteAction.rollbackFailed'),
                  description: error.message,
                  type: 'error',
                }),
            },
          )
        }}
      />
    </>
  )
}
