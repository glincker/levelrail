import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { dismissTour, TOUR_STEPS } from '@/lib/demoTour'

/** DemoTour is a short skippable walkthrough; closing it persists the dismissal. */
export function DemoTour({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation('demo')
  const [index, setIndex] = useState(0)
  const step = TOUR_STEPS[index] ?? TOUR_STEPS[0]
  const last = index === TOUR_STEPS.length - 1

  function close() {
    dismissTour()
    onClose()
  }

  return (
    <section
      aria-label={t(`tour.${step.id}.title`)}
      className="fixed bottom-4 left-4 z-40 w-[min(22rem,calc(100vw-2rem))] rounded-xl border border-border bg-popover p-4 text-popover-foreground shadow-lg motion-safe:animate-in motion-safe:fade-in-0"
    >
      <div className="flex items-start justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {t('tour.step', { current: index + 1, total: TOUR_STEPS.length })}
        </p>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={t('tour.skip')}
          onClick={close}
        >
          <XGlassIcon size="1.2em" />
        </Button>
      </div>
      <h2 className="mt-1 text-sm font-semibold">
        {t(`tour.${step.id}.title`)}
      </h2>
      <p className="mt-1 text-sm text-muted-foreground">
        {t(`tour.${step.id}.body`)}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          nativeButton={false}
          render={<Link to={step.to} />}
        >
          {t('tour.open')}
        </Button>
        <span className="ml-auto flex items-center gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={close}>
            {t('tour.skip')}
          </Button>
          {index > 0 ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setIndex(index - 1)
              }}
            >
              {t('tour.back')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            onClick={() => {
              if (last) close()
              else setIndex(index + 1)
            }}
          >
            {last ? t('tour.done') : t('tour.next')}
          </Button>
        </span>
      </div>
    </section>
  )
}
