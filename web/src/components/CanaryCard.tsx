import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GitForkGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import {
  useCanary,
  useCanaryActions,
  type CanaryStartBody,
} from '../queries/canary'
import { CanaryOutcomeNotice, CanaryProgressPanel } from './CanaryProgressPanel'

const DEFAULT_WEIGHT = 10

interface StagedForm {
  stages: string
  interval: string
  maxErrorPoints: string
  maxP95Ratio: string
}

function startBody(
  image: string,
  weight: string,
  form: StagedForm,
): CanaryStartBody {
  const stages = form.stages
    .split(',')
    .map((part) => Number(part.trim()))
    .filter((n) => Number.isFinite(n) && n > 0)
  if (stages.length === 0) return { image, weight: Number(weight) }
  const body: CanaryStartBody = { image, stages }
  if (form.interval.trim()) body.interval = form.interval.trim()
  if (Number(form.maxErrorPoints) > 0)
    body.max_error_rate = Number(form.maxErrorPoints) / 100
  if (Number(form.maxP95Ratio) > 1)
    body.max_p95_ratio = Number(form.maxP95Ratio)
  return body
}

/** CanaryCard sends a share of traffic to a new image before it replaces the running release. */
export function CanaryCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const { data: canary } = useCanary(appName)
  const { start, setWeight, promote, abort } = useCanaryActions(appName)
  const [image, setImage] = useState('')
  const [weight, setWeightInput] = useState(String(DEFAULT_WEIGHT))
  const [staged, setStaged] = useState<StagedForm>({
    stages: '',
    interval: '',
    maxErrorPoints: '',
    maxP95Ratio: '',
  })
  const isStaged = staged.stages.trim() !== ''

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
          <GitForkGlassIcon className="size-4.5 text-muted-foreground" />
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
            {canary.staged ? (
              <CanaryProgressPanel canary={canary} />
            ) : (
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
            )}
            <div className="flex gap-2 border-t border-border pt-3">
              <Button
                type="button"
                size="sm"
                disabled={busy}
                onClick={() =>
                  run(promote, undefined, t('canary.toast.promoted'))
                }
              >
                {t('canary.promoteNow')}
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
                startBody(image.trim(), weight, staged),
                t('canary.toast.started'),
              )
            }}
          >
            <div className="w-full">
              <CanaryOutcomeNotice canary={canary} />
            </div>
            <label className="min-w-48 flex-1 space-y-1 text-xs text-muted-foreground">
              {t('canary.imageLabel')}
              <Input
                value={image}
                onChange={(e) => setImage(e.target.value)}
                placeholder={t('canary.imagePlaceholder')}
              />
            </label>
            {isStaged ? null : (
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
            )}
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('canary.stagesLabel')}
              <Input
                value={staged.stages}
                onChange={(e) =>
                  setStaged({ ...staged, stages: e.target.value })
                }
                placeholder={t('canary.stagesPlaceholder')}
                className="w-40"
              />
            </label>
            {isStaged ? (
              <>
                <label className="space-y-1 text-xs text-muted-foreground">
                  {t('canary.intervalLabel')}
                  <Input
                    value={staged.interval}
                    onChange={(e) =>
                      setStaged({ ...staged, interval: e.target.value })
                    }
                    placeholder="5m"
                    className="w-20"
                  />
                </label>
                <label className="space-y-1 text-xs text-muted-foreground">
                  {t('canary.maxErrorLabel')}
                  <Input
                    type="number"
                    min={0}
                    step={0.5}
                    value={staged.maxErrorPoints}
                    onChange={(e) =>
                      setStaged({ ...staged, maxErrorPoints: e.target.value })
                    }
                    placeholder="2"
                    className="w-24"
                  />
                </label>
                <label className="space-y-1 text-xs text-muted-foreground">
                  {t('canary.maxP95Label')}
                  <Input
                    type="number"
                    min={1}
                    step={0.1}
                    value={staged.maxP95Ratio}
                    onChange={(e) =>
                      setStaged({ ...staged, maxP95Ratio: e.target.value })
                    }
                    placeholder="1.5"
                    className="w-24"
                  />
                </label>
              </>
            ) : null}
            <Button type="submit" size="sm" disabled={busy || !image.trim()}>
              {t('canary.start')}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
