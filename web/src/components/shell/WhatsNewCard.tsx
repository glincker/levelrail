import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRightGlassIcon,
  BoxGlassIcon,
  DatabaseGlassIcon,
  GlobeGlassIcon,
  SparklesGlassIcon,
  ZapGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { ParsedBullet, WhatsNewGroup } from './whatsNewHighlights'

const GROUP_ICON = {
  apps: BoxGlassIcon,
  domains: GlobeGlassIcon,
  storage: DatabaseGlassIcon,
  nextjs: ZapGlassIcon,
  other: SparklesGlassIcon,
} satisfies Record<WhatsNewGroup, typeof BoxGlassIcon>

const ROUTE_CTA = {
  '/domains': 'domains',
  '/storage': 'storage',
  '/apps': 'apps',
  '/deployments': 'deployments',
  '/settings': 'settings',
} as const

export function WhatsNewCard({
  item,
  onNavigate,
}: {
  item: ParsedBullet
  onNavigate: () => void
}) {
  const { t } = useTranslation('common', { useSuspense: false })
  const navigate = useNavigate()
  const Icon = GROUP_ICON[item.group]
  const detail = item.detail || t(`shell.whatsNew.groups.${item.group}`)
  return (
    <li className="flex flex-col gap-2 rounded-lg border bg-card p-3 transition-colors hover:bg-muted/60 motion-reduce:transition-none">
      <div className="flex items-start gap-2.5">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-tone-info-soft text-tone-info">
          <Icon size="1.4em" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <p className="line-clamp-2 text-sm font-medium" title={item.text}>
            {item.title}
          </p>
          <p className="line-clamp-2 text-xs text-muted-foreground">{detail}</p>
        </div>
      </div>
      {item.route ? (
        <Button
          type="button"
          variant="outline"
          className="h-10 cursor-pointer justify-between px-3 max-sm:w-full"
          onClick={() => {
            onNavigate()
            void navigate({ to: item.route ?? '/' })
          }}
        >
          {t(`shell.whatsNew.cta.${ROUTE_CTA[item.route]}`)}
          <ArrowRightGlassIcon size="1.2em" aria-hidden="true" />
        </Button>
      ) : null}
    </li>
  )
}
