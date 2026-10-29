import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import {
  BoxGlassIcon,
  ComputerGlassIcon,
  GlobeGlassIcon,
  LayersGlassIcon,
  TerminalGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { ImportFrontDoor } from '../ImportFrontDoor'
import { SitePublisher } from '../dropdeploy/SitePublisher'
import type { ImportPlan, ImportPlanRequest } from '../../queries/imports'
import { DeployFromComputer } from './DeployFromComputer'
import { SectionHeading } from './SectionHeading'

type WayKey = 'image' | 'compose' | 'run' | 'site' | 'computer'

const WAYS: { key: WayKey; icon: UiIcon }[] = [
  { key: 'image', icon: BoxGlassIcon },
  { key: 'compose', icon: LayersGlassIcon },
  { key: 'run', icon: TerminalGlassIcon },
  { key: 'site', icon: GlobeGlassIcon },
  { key: 'computer', icon: ComputerGlassIcon },
]

/**
 * One row of non-git starts. Picking a card opens its form inline below the
 * row; only one is open at a time.
 */
export function MoreWays({
  onPlan,
}: {
  onPlan: (plan: ImportPlan, request: ImportPlanRequest) => void
}) {
  const { t } = useTranslation('newApp')
  const navigate = useNavigate()
  const [open, setOpen] = useState<WayKey | null>(null)

  return (
    <section
      aria-labelledby="new-app-more"
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-xs"
    >
      <SectionHeading id="new-app-more">{t('more.title')}</SectionHeading>
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
        {WAYS.map(({ key, icon: WayIcon }) => (
          <li
            key={key}
            className={
              key === 'computer' ? 'col-span-2 sm:col-span-1' : undefined
            }
          >
            <button
              type="button"
              aria-expanded={open === key}
              aria-controls="new-app-more-panel"
              onClick={() => {
                setOpen(open === key ? null : key)
              }}
              className={cn(
                'flex h-full w-full items-start gap-2.5 rounded-lg border border-border p-2.5 text-left transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                open === key && 'border-primary/50 bg-muted/50',
              )}
            >
              <WayIcon
                className="mt-0.5 size-6 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span className="min-w-0">
                <span className="block text-sm font-medium text-foreground">
                  {t(`more.${key}.title`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`more.${key}.hint`)}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
      {open ? (
        <div
          id="new-app-more-panel"
          className="space-y-2 rounded-lg border border-border p-3"
        >
          {open === 'site' ? (
            <SitePublisher
              onPublished={(appName, result) => {
                toast.add({
                  title: t('toast.created', { name: appName }),
                  description: t('toast.watching'),
                  type: 'success',
                })
                if (result.deployId) {
                  void navigate({
                    to: '/apps/$name/deploys/$deployId/logs',
                    params: { name: appName, deployId: result.deployId },
                  })
                }
              }}
            />
          ) : open === 'computer' ? (
            <DeployFromComputer />
          ) : (
            <ImportFrontDoor onPlan={onPlan} />
          )}
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setOpen(null)
            }}
          >
            {t('more.hide')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}
