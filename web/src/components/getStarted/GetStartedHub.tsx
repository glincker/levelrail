import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { ProgressRing } from '@/components/ui/progress-ring'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import { essentialSplit, isOpen, stepCount } from '../../lib/setupGuide'
import type {
  SetupGuide,
  SetupGuideOverride,
  SetupGuideStep,
} from '../../lib/setupGuide'
import {
  useResetGuide,
  useSetGuideHidden,
  useSetGuideStep,
  useSetupGuide,
} from '../../queries/setupGuide'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { RuntimeSettingsReview } from './RuntimeSettingsReview'
import { SampleAppButton } from './SampleAppButton'
import { StepCard } from './StepCard'

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

export function GetStartedHub() {
  const { t } = useTranslation('getStarted')
  const {
    data: guide,
    error,
    isPending,
    refetch,
  } = useSetupGuide({
    live: true,
  })

  if (isPending) return <ListSkeleton rows={5} />
  if (error || !guide) {
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-wrap items-center gap-3">
          {t('page.loadError', {
            message: error instanceof Error ? error.message : '',
          })}
          <Button size="sm" variant="outline" onClick={() => void refetch()}>
            {t('page.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  return <GuideBody guide={guide} />
}

export function GuideBody({ guide }: { guide: SetupGuide }) {
  const { t } = useTranslation('getStarted')
  const isRoot = useIsRoot()
  const setStep = useSetGuideStep()
  const setHidden = useSetGuideHidden()
  const reset = useResetGuide()
  const [confirmReset, setConfirmReset] = useState(false)
  const busy = setStep.isPending || setHidden.isPending || reset.isPending
  const { essential, more } = essentialSplit(guide)
  const { done, total, percent } = stepCount(guide)
  const moreOpen = more.filter(isOpen)
  const moreSettled = more.filter((s) => !isOpen(s))

  const onError = (err: unknown) => {
    toast.add({
      title: t('actions.saveError', { message: errorMessage(err) }),
      type: 'error',
    })
  }
  const override = (id: SetupGuideStep['id'], state: SetupGuideOverride) => {
    setStep.mutate({ id, state }, { onError })
  }
  const card = (step: SetupGuideStep, highlighted = false) => (
    <StepCard
      key={step.id}
      step={step}
      highlighted={highlighted}
      canEdit={isRoot}
      busy={busy}
      primaryAction={
        step.id === 'first_app' ? (
          <>
            <CreateResourceWizard
              trigger={
                <Button
                  size="sm"
                  variant={highlighted || isOpen(step) ? 'default' : 'outline'}
                >
                  {t('steps.first_app.cta')}
                </Button>
              }
            />
            <SampleAppButton />
          </>
        ) : undefined
      }
      onSkip={() => {
        override(step.id, 'skipped')
      }}
      onUndo={() => {
        override(step.id, 'todo')
      }}
      onMarkDone={() => {
        override(step.id, 'done')
      }}
    />
  )

  return (
    <div className="space-y-6">
      <section
        aria-label={t('summary.ready', { percent: guide.ready_percent })}
        className="flex flex-wrap items-center gap-x-6 gap-y-3 rounded-lg border border-border bg-card px-4 py-3"
      >
        <ProgressRing
          value={percent}
          label={t('chip.steps', { done, total })}
          display={`${done}/${total}`}
          className="size-12"
        />
        <div className="min-w-0 flex-1">
          <p className="text-base font-semibold text-foreground">
            {t('chip.steps', { done, total })}
          </p>
          <p className="text-xs text-muted-foreground tabular-nums">
            {t('summary.ready', { percent: guide.ready_percent })}
          </p>
        </div>
        {isRoot ? (
          <div className="flex shrink-0 gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => {
                setHidden.mutate(!guide.hidden, { onError })
              }}
            >
              {guide.hidden ? t('actions.show') : t('actions.hide')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={() => {
                setConfirmReset(true)
              }}
            >
              {t('actions.reset')}
            </Button>
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">{t('page.rootOnly')}</p>
        )}
        {guide.server_address ? (
          <p className="basis-full text-xs text-muted-foreground">
            {t('summary.serverAddress', { address: guide.server_address })}
          </p>
        ) : null}
      </section>

      {guide.hidden ? (
        <p className="text-sm text-muted-foreground">{t('actions.hidden')}</p>
      ) : null}

      {guide.complete ? (
        <EmptyState
          icon={<RocketGlassIcon className="size-8" />}
          title={t('summary.allDoneTitle')}
          description={t('summary.allDoneBody')}
        />
      ) : null}

      <RuntimeSettingsReview />

      <section className="space-y-2">
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          {t('summary.essentialTitle')}
        </h2>
        <ul className="space-y-2">
          {essential.map((s) => card(s, s.id === guide.next_step))}
        </ul>
      </section>

      {moreOpen.length > 0 ? (
        <section className="space-y-2">
          <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            {t('more.title')}
          </h2>
          <ul className="space-y-2">
            {moreOpen.map((s) => card(s, s.id === guide.next_step))}
          </ul>
        </section>
      ) : null}

      {moreSettled.length > 0 ? (
        <Collapsible defaultOpen={false}>
          <CollapsibleTrigger className="flex items-center gap-2 rounded-md px-1 py-1 text-sm font-medium text-muted-foreground hover:text-foreground">
            {t('more.finished', { count: moreSettled.length })}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <ul className="space-y-2 pt-2">
              {moreSettled.map((s) => card(s))}
            </ul>
          </CollapsiblePanel>
        </Collapsible>
      ) : null}

      <ConfirmDialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={t('actions.resetConfirmTitle')}
        description={t('actions.resetConfirmBody')}
        confirmLabel={t('actions.reset')}
        pending={reset.isPending}
        onConfirm={() => {
          reset.mutate(undefined, {
            onSuccess: () => {
              setConfirmReset(false)
            },
            onError,
          })
        }}
      />
    </div>
  )
}
