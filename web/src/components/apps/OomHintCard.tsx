import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useApp, useUpdateApp } from '../../queries/apps'
import { useResourceRecommendation } from '../../queries/resourceRecommendation'
import { formatBytes } from '../../lib/format'

// Shown on the app overview after an out-of-memory kill, with a one-click raise
// to the advisor's suggested limit.
export function OomHintCard({ appName }: { appName: string }) {
  const { t } = useTranslation('lifecycle')
  const { data: app } = useApp(appName)
  const { data } = useResourceRecommendation(appName)
  const update = useUpdateApp(appName)
  if (!data?.oom_detected_at) return null
  const mem = data.memory
  const suggested = mem.suggested_limit
  const canRaise = mem.action === 'raise' && suggested > mem.current_limit
  const date = new Date(data.oom_detected_at).toLocaleString()
  const limit = formatBytes(mem.current_limit)
  return (
    <Alert variant="destructive">
      <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
      <AlertDescription className="space-y-2">
        <p className="font-medium">{t('oom.title')}</p>
        <p>
          {mem.p99_usage > 0
            ? t('oom.body', { limit, date, peak: formatBytes(mem.p99_usage) })
            : t('oom.bodyNoPeak', { limit, date })}
        </p>
        {canRaise ? (
          <Button
            size="sm"
            disabled={update.isPending}
            onClick={() => {
              update.mutate(
                {
                  ...app,
                  resources: { ...app.resources, memory_bytes: suggested },
                },
                {
                  onSuccess: () => {
                    toast.add({
                      title: t('oom.raised', { value: formatBytes(suggested) }),
                      type: 'success',
                    })
                  },
                  onError: (err) => {
                    toast.add({
                      title: t('oom.raiseFailed', { message: err.message }),
                      type: 'error',
                    })
                  },
                },
              )
            }}
          >
            {update.isPending
              ? t('oom.raising')
              : t('oom.raise', { value: formatBytes(suggested) })}
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
