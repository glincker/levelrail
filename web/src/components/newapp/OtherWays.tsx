import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CubeIcon,
  StackIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ImportFrontDoor } from '../ImportFrontDoor'
import type { ImportPlan, ImportPlanRequest } from '../../queries/imports'

type WayKey = 'image' | 'compose' | 'run'

const WAYS: { key: WayKey; icon: Icon }[] = [
  { key: 'image', icon: CubeIcon },
  { key: 'compose', icon: StackIcon },
  { key: 'run', icon: TerminalWindowIcon },
]

/**
 * Compact cards for the non-git sources. All three feed the same import
 * front door, which tells image, compose and docker run apart by itself.
 */
export function OtherWays({
  onPlan,
}: {
  onPlan: (plan: ImportPlan, request: ImportPlanRequest) => void
}) {
  const { t } = useTranslation('newApp')
  const [open, setOpen] = useState(false)
  const label = (key: WayKey) =>
    key === 'image'
      ? t('source.other.image')
      : key === 'compose'
        ? t('source.other.compose')
        : t('source.other.run')
  const hint = (key: WayKey) =>
    key === 'image'
      ? t('source.other.imageHint')
      : key === 'compose'
        ? t('source.other.composeHint')
        : t('source.other.runHint')

  return (
    <section aria-labelledby="new-app-other" className="space-y-3">
      <div>
        <h2
          id="new-app-other"
          className="text-base font-medium text-foreground"
        >
          {t('source.other.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('source.other.description')}
        </p>
      </div>
      <ul className="grid gap-2 sm:grid-cols-3">
        {WAYS.map(({ key, icon: WayIcon }) => (
          <li key={key}>
            <button
              type="button"
              aria-expanded={open}
              onClick={() => {
                setOpen(true)
              }}
              className="flex h-full w-full items-start gap-2.5 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <WayIcon
                className="mt-0.5 size-5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span className="min-w-0">
                <span className="block text-sm font-medium text-foreground">
                  {label(key)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {hint(key)}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
      {open ? (
        <div className="space-y-2 rounded-lg border border-border p-3">
          <ImportFrontDoor onPlan={onPlan} />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setOpen(false)
            }}
          >
            {t('source.other.hide')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}
