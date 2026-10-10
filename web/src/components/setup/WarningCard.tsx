import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  CaretDownIcon,
  QuestionIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button, buttonVariants } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import { CheckDetail } from '../CheckDetail'
import { HostFirewallToggle } from '../HostFirewallToggle'
import { joinPorts } from '../../lib/setupGuidance'
import type { WarningGroup } from '../../lib/setupGuidance'

/** WarningCard turns a failing or warning check (or a group of related ones) into a decision: a one-line why, a primary action, and the rest behind a toggle. */
export function WarningCard({
  group,
  blocking,
}: {
  group: WarningGroup
  blocking: boolean
}) {
  const { t } = useTranslation('setup')
  const [explainerOpen, setExplainerOpen] = useState(false)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const { guidance, checks, ports } = group
  const first = checks[0]
  if (!first) return null
  const tone = TONE[blocking ? 'danger' : 'warning']
  const Icon = blocking ? XCircleIcon : WarningCircleIcon
  const base = guidance ? (`warnings.guidance.${guidance.key}` as const) : null
  const vars = { ports: joinPorts(ports), first: ports[0] ?? '' }
  const grouped = checks.length > 1
  const title =
    guidance?.key === 'reachability' && grouped
      ? t('warnings.groupReachability', { ports: vars.ports })
      : guidance?.key === 'port' && grouped
        ? t('warnings.groupPort', { ports: vars.ports })
        : first.name
  const action = guidance?.action
  const hasMore = Boolean(base) || grouped || first.code === 'firewall'

  return (
    <article
      className={cn('rounded-xl border bg-card p-4', tone.border)}
      aria-label={title}
    >
      <header className="flex items-start gap-3">
        <span
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-lg',
            tone.soft,
            tone.text,
          )}
        >
          <Icon className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground">{title}</h3>
          <p className="mt-0.5 break-words text-sm text-muted-foreground">
            {base ? t(`${base}.why`, vars) : first.message}
          </p>
        </div>
      </header>

      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 pl-11">
        {action?.kind === 'link' && base ? (
          <Link to={action.to} className={buttonVariants({ size: 'sm' })}>
            {t(`${base}.action`, vars)}
            <ArrowRightIcon />
          </Link>
        ) : null}
        {action?.kind === 'explainer' ? (
          <Button
            type="button"
            size="sm"
            aria-expanded={explainerOpen}
            onClick={() => setExplainerOpen((v) => !v)}
          >
            <QuestionIcon />
            {explainerOpen
              ? t('warnings.explainerClose')
              : t('warnings.guidance.reachability.action', vars)}
          </Button>
        ) : null}
        {action?.kind === 'docs' && base ? (
          <HelpLink
            path={action.path}
            label={t(`${base}.action`, vars)}
            variant="inline"
          />
        ) : null}
        {!guidance ? (
          <Link
            to="/settings/system-status"
            className={buttonVariants({ size: 'sm', variant: 'outline' })}
          >
            {t('warnings.openStatus')}
            <ArrowRightIcon />
          </Link>
        ) : null}
        <button
          type="button"
          aria-expanded={detailsOpen}
          onClick={() => setDetailsOpen((v) => !v)}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          {detailsOpen ? t('warnings.hideDetails') : t('warnings.showDetails')}
          <CaretDownIcon
            className={cn(
              'size-3 transition-transform motion-reduce:transition-none',
              detailsOpen && 'rotate-180',
            )}
            aria-hidden="true"
          />
        </button>
      </div>

      {action?.kind === 'explainer' && explainerOpen ? (
        <p className="kit-enter mt-3 ml-11 rounded-lg border border-border bg-muted/50 p-3 text-sm text-foreground">
          {t('warnings.guidance.reachability.explainer', vars)}
        </p>
      ) : null}

      {detailsOpen ? (
        <div className="kit-enter mt-3 ml-11 space-y-3 border-t border-border pt-3">
          {base ? (
            <p className="text-sm text-foreground">
              <span className="font-medium">{t('warnings.ifIgnored')}: </span>
              {t(`${base}.ifIgnored`, vars)}
            </p>
          ) : null}
          {checks.map((check) => (
            <div key={check.code} className="space-y-1">
              {hasMore && (grouped || base) ? (
                <p className="text-xs">
                  <span className="font-medium text-foreground">
                    {check.name}
                  </span>
                  <span className="block break-words text-muted-foreground">
                    {check.message}
                  </span>
                </p>
              ) : null}
              <CheckDetail check={check} />
            </div>
          ))}
          {first.code === 'firewall' ? <HostFirewallToggle /> : null}
        </div>
      ) : null}
    </article>
  )
}
