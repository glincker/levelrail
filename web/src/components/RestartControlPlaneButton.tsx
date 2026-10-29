import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useIsRoot } from '../hooks/useIsRoot'
import { useRestartState } from '../lib/restartTracker'
import {
  useRestartControlPlane,
  useSystemRestartInfo,
} from '../queries/systemRestart'

// Renders nothing for non-root users or when this install cannot
// self-restart; callers keep their manual-command hint as the fallback.
export function RestartControlPlaneButton({
  variant = 'outline',
  label,
}: {
  variant?: 'outline' | 'default'
  label?: string
}) {
  const { t } = useTranslation('common')
  const isRoot = useIsRoot()
  const { data: info } = useSystemRestartInfo()
  const restart = useRestartControlPlane()
  const tracking = useRestartState()
  const [open, setOpen] = useState(false)

  if (!isRoot || !info?.supported) return null
  const busy = restart.isPending || tracking?.phase === 'restarting'

  return (
    <>
      <Button variant={variant} disabled={busy} onClick={() => setOpen(true)}>
        <RefreshCwGlassIcon className="size-4.5" aria-hidden="true" />
        {label ?? t('controlPlaneRestart.action')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('controlPlaneRestart.title')}
        description={t('controlPlaneRestart.description')}
        consequences={[
          t('controlPlaneRestart.consequence1'),
          t('controlPlaneRestart.consequence2'),
        ]}
        confirmLabel={label ?? t('controlPlaneRestart.action')}
        pending={restart.isPending}
        error={restart.error?.message ?? null}
        onConfirm={() =>
          restart.mutate(undefined, { onSuccess: () => setOpen(false) })
        }
      />
    </>
  )
}
