import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { formatSize } from '../lib/format'
import { formatRelativeTime } from '../lib/relativeTime'
import {
  useAppBuildCache,
  useClearAppBuildCache,
} from '../queries/appBuildCache'

export function AppBuildCacheRow({ appName }: { appName: string }) {
  const { t } = useTranslation('appBuildCache')
  const [confirming, setConfirming] = useState(false)
  const { data, error } = useAppBuildCache(appName)
  const clear = useClearAppBuildCache(appName)

  // 501 means this control plane has no builder; the row would only confuse.
  if (error || !data) return null

  const empty = data.record_count === 0

  function onConfirm() {
    clear.mutate(undefined, {
      onSuccess: (res) => {
        setConfirming(false)
        toast.add({
          title: t('cleared', { size: formatSize(res.freed_bytes) }),
          type: 'success',
        })
      },
    })
  }

  return (
    <section className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
      <p className="flex items-center gap-2 text-sm">
        <DatabaseGlassIcon aria-hidden="true" className="size-5" />
        <span className="font-medium">{t('label')}</span>
        <span className="text-muted-foreground">
          {empty
            ? t('empty')
            : t('summary', {
                size: formatSize(data.size_bytes),
                used: data.last_used_at
                  ? formatRelativeTime(data.last_used_at)
                  : t('neverUsed'),
              })}
        </span>
      </p>
      {empty ? null : (
        <Button
          size="sm"
          variant="outline"
          disabled={data.in_use}
          onClick={() => setConfirming(true)}
        >
          {t('clear')}
        </Button>
      )}
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open) {
            setConfirming(false)
            clear.reset()
          }
        }}
        title={t('confirmTitle')}
        description={t('confirmBody')}
        confirmLabel={t('clear')}
        pendingLabel={t('clearing')}
        pending={clear.isPending}
        error={clear.error?.message ?? null}
        onConfirm={onConfirm}
      />
    </section>
  )
}
