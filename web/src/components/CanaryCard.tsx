import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GitForkIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import { useCanary, useCanaryActions } from '../queries/canary'

const DEFAULT_WEIGHT = 10

/** CanaryCard sends a share of traffic to a new image before it replaces the running release. */
export function CanaryCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const { data: canary } = useCanary(appName)
  const { start, setWeight, promote, abort } = useCanaryActions(appName)
  const [image, setImage] = useState('')
  const [weight, setWeightInput] = useState(String(DEFAULT_WEIGHT))

  const busy =
    start.isPending ||
    setWeight.isPending ||
    promote.isPending ||
    abort.isPending

  function run<T>(
    mutation: { mutate: (v: T, o: object) => void },
    value: T,
    success: string,
  ) {
    mutation.mutate(value, {
      onSuccess: () => toast.add({ title: success, type: 'success' }),
      onError: (error: Error) =>
        toast.add({
          title: t('canary.toast.errorTitle'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitForkIcon className="size-4 text-muted-foreground" />
          {t('canary.title')}
          <InfoTip label={t('canary.infoTipLabel')}>
            {t('canary.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {canary.active ? (
          <>
            <p className="text-sm text-foreground" role="status">
              {t('canary.running', {
                image: canary.image,
                weight: canary.weight,
              })}
            </p>
            <div className="flex items-end gap-2">
              <label className="space-y-1 text-xs text-muted-foreground">
                {t('canary.weightLabel')}
                <Input
                  type="number"
                  min={0}
                  max={99}
                  value={weight}
                  onChange={(e) => setWeightInput(e.target.value)}
                  className="w-24"
                />
              </label>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() =>
                  run(setWeight, Number(weight), t('canary.toast.weightSet'))
                }
              >
                {t('canary.setWeight')}
              </Button>
            </div>
            <div className="flex gap-2 border-t border-border pt-3">
              <Button
                type="button"
                size="sm"
                disabled={busy}
                onClick={() =>
                  run(promote, undefined, t('canary.toast.promoted'))
                }
              >
                {t('canary.promote')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => run(abort, undefined, t('canary.toast.aborted'))}
              >
                {t('canary.abort')}
              </Button>
            </div>
          </>
        ) : (
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              run(
                start,
                { image: image.trim(), weight: Number(weight) },
                t('canary.toast.started'),
              )
            }}
          >
            <label className="min-w-48 flex-1 space-y-1 text-xs text-muted-foreground">
              {t('canary.imageLabel')}
              <Input
                value={image}
                onChange={(e) => setImage(e.target.value)}
                placeholder={t('canary.imagePlaceholder')}
              />
            </label>
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('canary.weightLabel')}
              <Input
                type="number"
                min={1}
                max={99}
                value={weight}
                onChange={(e) => setWeightInput(e.target.value)}
                className="w-24"
              />
            </label>
            <Button type="submit" size="sm" disabled={busy || !image.trim()}>
              {t('canary.start')}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
