import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useRestartState } from '../../lib/restartTracker'
import {
  useApplyRuntimeSettings,
  useRuntimeSettings,
} from '../../queries/runtimeSettings'

export function RuntimeRestartBanner({
  count,
  supported,
  canEdit,
}: {
  count: number
  supported: boolean
  canEdit: boolean
}) {
  const { t } = useTranslation('runtimeSettings')
  const apply = useApplyRuntimeSettings()
  const { refetch, isFetching } = useRuntimeSettings()
  const tracking = useRestartState()
  const [open, setOpen] = useState(false)
  const busy = apply.isPending || tracking?.phase === 'restarting'

  let action = null
  if (canEdit && supported) {
    action = (
      <Button
        type="button"
        size="sm"
        disabled={busy}
        onClick={() => {
          setOpen(true)
        }}
      >
        <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
        {t('restart.now')}
      </Button>
    )
  } else if (canEdit) {
    action = (
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={isFetching}
        onClick={() => void refetch()}
      >
        <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
        {t('restart.recheck')}
      </Button>
    )
  }

  return (
    <>
      <Banner
        variant="warning"
        title={t('restart.title', { count })}
        action={action}
      >
        {supported ? t('restart.body') : t('restart.unsupported')}
      </Banner>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('restart.confirmTitle')}
        description={t('restart.confirmBody')}
        confirmLabel={t('restart.now')}
        pending={apply.isPending}
        error={
          apply.error
            ? t('restart.error', { message: apply.error.message })
            : null
        }
        onConfirm={() => {
          apply.mutate(undefined, {
            onSuccess: () => {
              setOpen(false)
            },
          })
        }}
      />
    </>
  )
}
