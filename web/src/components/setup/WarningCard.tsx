import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
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
import { guidanceFor } from '../../lib/setupGuidance'
import type { DoctorCheck } from '../../queries/systemDoctor'

/** WarningCard turns one failing or warning check into a decision: why it matters, what happens if ignored, and a primary action. */
export function WarningCard({
  check,
  blocking,
}: {
  check: DoctorCheck
  blocking: boolean
}) {
  const { t } = useTranslation('setup')
  const [explainerOpen, setExplainerOpen] = useState(false)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const guidance = guidanceFor(check)
  const tone = TONE[blocking ? 'danger' : 'warning']
  const Icon = blocking ? XCircleIcon : WarningCircleIcon
  const base = `warnings.guidance.${guidance.key}` as const
  const port = { port: guidance.port ?? '' }
  const action = guidance.action

  return (
    <article
      className={cn('rounded-xl border bg-card p-4', tone.border)}
      aria-label={check.name}
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
        <div className="min-w-0">
          <h3 className="text-sm font-medium text-foreground">{check.name}</h3>
          <p className="break-words text-xs text-muted-foreground">
            {check.message}
          </p>
        </div>
      </header>

      <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t('warnings.why')}
          </dt>
          <dd className="mt-1 text-foreground">{t(`${base}.why`, port)}</dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t('warnings.ifIgnored')}
          </dt>
          <dd className="mt-1 text-foreground">
            {t(`${base}.ifIgnored`, port)}
          </dd>
        </div>
      </dl>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        {action.kind === 'link' ? (
          <Link to={action.to} className={buttonVariants({ size: 'sm' })}>
            {t(`${base}.action`, port)}
            <ArrowRightIcon />
          </Link>
        ) : null}
        {action.kind === 'explainer' ? (
          <Button
            type="button"
            size="sm"
            aria-expanded={explainerOpen}
            onClick={() => setExplainerOpen((v) => !v)}
          >
            <QuestionIcon />
            {explainerOpen
              ? t('warnings.explainerClose')
              : t('warnings.guidance.reachability.action', port)}
          </Button>
        ) : null}
        {action.kind === 'docs' ? (
          <HelpLink
            path={action.path}
            label={t(`${base}.action`, port)}
            variant="inline"
          />
        ) : null}
      </div>

      {action.kind === 'explainer' && explainerOpen ? (
        <p className="kit-enter mt-3 rounded-lg border border-border bg-muted/50 p-3 text-sm text-foreground">
          {t('warnings.guidance.reachability.explainer', port)}
        </p>
      ) : null}

      {check.code === 'firewall' ? (
        <div className="mt-3">
          <HostFirewallToggle />
        </div>
      ) : null}
      <button
        type="button"
        aria-expanded={detailsOpen}
        onClick={() => setDetailsOpen((v) => !v)}
        className="mt-3 text-xs text-muted-foreground underline underline-offset-4 outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        {detailsOpen ? t('warnings.hideDetails') : t('warnings.showDetails')}
      </button>
      {detailsOpen ? <CheckDetail check={check} /> : null}
    </article>
  )
}
