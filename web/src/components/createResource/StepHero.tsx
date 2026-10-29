import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import type { EngineAccent } from '../../lib/engineAccents'

export interface StepHeroProps {
  /** The logo tile content: a brand mark or an icon. */
  mark: ReactNode
  title: string
  description: string
  chips?: string[]
  accent: EngineAccent
  onBack: () => void
}

// Header of the second creation step: the picked starting point's logo at the
// left, a soft brand-coloured wash and a short entrance that reduced-motion
// users do not get.
export function StepHero({
  mark,
  title,
  description,
  chips = [],
  accent,
  onBack,
}: StepHeroProps) {
  const { t } = useTranslation('databases')
  return (
    <div className="flex flex-col gap-3">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="-ml-2 w-fit text-muted-foreground"
        onClick={onBack}
      >
        <ArrowLeftGlassIcon size="1.4em" aria-hidden="true" />
        {t('wizard.back')}
      </Button>
      <div
        className={cn(
          'relative isolate flex items-center gap-4 overflow-hidden rounded-xl border border-border bg-gradient-to-br to-transparent p-4',
          'animate-in fade-in-0 slide-in-from-bottom-1 duration-200 motion-reduce:animate-none',
          accent.wash,
        )}
      >
        <span
          aria-hidden="true"
          className={cn(
            'pointer-events-none absolute -top-10 -right-8 -z-10 size-40 rounded-full blur-3xl',
            accent.orb,
          )}
        />
        <span
          className={cn(
            'flex size-14 shrink-0 items-center justify-center rounded-xl bg-white text-neutral-900 shadow-sm ring-1',
            accent.ring,
          )}
        >
          {mark}
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <DialogTitle className="text-lg leading-tight font-semibold">
            {title}
          </DialogTitle>
          <DialogDescription className="text-sm text-muted-foreground">
            {description}
          </DialogDescription>
          {chips.length > 0 ? (
            <div className="flex flex-wrap gap-1.5 pt-1">
              {chips.map((chip) => (
                <Badge key={chip} variant="muted" className="font-normal">
                  {chip}
                </Badge>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}
