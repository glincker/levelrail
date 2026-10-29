import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import { healthCheckFrom } from '../../lib/healthCheckDefaults'
import { useCreateApp } from '../../queries/apps'
import { SAMPLE_APP } from '../../lib/firstApp'

const HTTP_CONFLICT = 409

/** SampleAppButton deploys the small public sample image in one click. */
export function SampleAppButton() {
  const { t } = useTranslation('getStarted')
  const createApp = useCreateApp()

  const deploy = () => {
    createApp.mutate(
      {
        name: SAMPLE_APP.name,
        image: SAMPLE_APP.image,
        port: SAMPLE_APP.port,
        health: healthCheckFrom(true, SAMPLE_APP.readinessPath),
      },
      {
        onSuccess: () => {
          toast.add({ title: t('sample.created'), type: 'success' })
        },
        onError: (err) => {
          if (err instanceof ApiError && err.status === HTTP_CONFLICT) {
            toast.add({ title: t('sample.exists'), type: 'info' })
            return
          }
          toast.add({
            title: t('sample.error', { message: err.message }),
            type: 'error',
          })
        },
      },
    )
  }

  if (createApp.isSuccess) {
    return (
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/apps/$name" params={{ name: SAMPLE_APP.name }} />}
        nativeButton={false}
      >
        {t('sample.open')}
      </Button>
    )
  }

  return (
    <Button
      size="sm"
      variant="outline"
      disabled={createApp.isPending}
      onClick={deploy}
    >
      <RocketGlassIcon className="size-5" />
      {createApp.isPending ? t('sample.creating') : t('sample.cta')}
    </Button>
  )
}
