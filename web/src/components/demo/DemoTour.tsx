import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { MapGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { captureDemoEvent } from '@/lib/demoAnalytics'
import { dismissTour, TOUR_STEPS } from '@/lib/demoTour'
import { DemoStarButton } from './DemoBanner'

/** DemoTour is a launcher chip that opens a five step popover tour; it never opens by itself. */
export function DemoTour({
  onDismiss,
  onInstall,
}: {
  onDismiss: () => void
  onInstall: () => void
}) {
  const { t } = useTranslation('demo')
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [index, setIndex] = useState(0)
  const step = TOUR_STEPS[index] ?? TOUR_STEPS[0]
  const last = index === TOUR_STEPS.length - 1

  function go(next: number) {
    const target = TOUR_STEPS[next]
    if (!target) return
    setIndex(next)
    captureDemoEvent('demo_tour_step', {
      step: target.id,
      index: String(next + 1),
    })
    if (target.to) void navigate({ to: target.to })
  }

  function close() {
    dismissTour()
    setOpen(false)
    onDismiss()
  }

  return (
    <div className="fixed right-4 bottom-4 z-40">
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (next) go(index)
        }}
      >
        <PopoverTrigger
          render={
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="rounded-full bg-popover shadow-md"
            />
          }
        >
          <MapGlassIcon size="1.4em" data-icon="inline-start" />
          {t('tour.launcher')}
        </PopoverTrigger>
        <PopoverContent side="top" align="end" className="w-80 p-3">
          <p className="text-xs text-muted-foreground">
            {t('tour.step', { current: index + 1, total: TOUR_STEPS.length })}
          </p>
          <h2 className="text-sm font-semibold">
            {t(`tour.${step.id}.title`)}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t(`tour.${step.id}.body`)}
          </p>
          {last ? (
            <div className="flex items-center gap-2">
              <DemoStarButton />
              <Button
                type="button"
                size="xs"
                variant="outline"
                onClick={() => {
                  setOpen(false)
                  onInstall()
                }}
              >
                {t('banner.install')}
              </Button>
            </div>
          ) : null}
          <div className="flex items-center gap-1">
            <Button type="button" variant="ghost" size="sm" onClick={close}>
              {t('tour.skip')}
            </Button>
            <span className="ml-auto flex items-center gap-1">
              {index > 0 ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    go(index - 1)
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
                  else go(index + 1)
                }}
              >
                {last ? t('tour.done') : t('tour.next')}
              </Button>
            </span>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  )
}
