import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import type { ProbeTarget } from '../../queries/appExposure'
import { ProbeNote, ScopeChip, SecurityChip } from './ReachParts'
import { useCopyAddress } from './useCopyAddress'
import { resolveSecurity, type ReachAddress } from './reachModel'

type HeroStatus = 'live' | 'down' | 'unchecked' | 'checking' | 'stopped'

function StatusPill({ status }: { status: HeroStatus }) {
  const { t } = useTranslation('appAccess')
  const map = {
    live: { variant: 'success', Icon: CircleCheckGlassIcon },
    down: { variant: 'warning', Icon: TriangleAlertGlassIcon },
    unchecked: { variant: 'muted', Icon: CircleDashedGlassIcon },
    checking: { variant: 'muted', Icon: LoaderCircleGlassIcon },
    stopped: { variant: 'muted', Icon: CircleDashedGlassIcon },
  } as const
  const { variant, Icon } = map[status]
  return (
    <Badge variant={variant} role="status">
      <Icon
        aria-hidden="true"
        className={
          status === 'checking' ? 'animate-spin motion-reduce:animate-none' : ''
        }
      />
      {t(`status.${status}`)}
    </Badge>
  )
}

interface ReachHeroProps {
  addr: ReachAddress
  appName: string
  running: boolean
  probe?: ProbeTarget
  checking: boolean
  onRecheck: () => void
  onAllow: () => void
}

/** The best address to open, with its live status and TLS state. */
export function ReachHero({
  addr,
  appName,
  running,
  probe,
  checking,
  onRecheck,
  onAllow,
}: ReachHeroProps) {
  const { t } = useTranslation('appAccess')
  const { copied, copy } = useCopyAddress()
  const status: HeroStatus = !running
    ? 'stopped'
    : checking
      ? 'checking'
      : probe
        ? probe.ok
          ? 'live'
          : 'down'
        : 'unchecked'
  return (
    <section
      aria-label={t('hero.title')}
      className="space-y-3 rounded-lg border border-primary/20 bg-primary/5 p-4"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-sm font-semibold">{t('hero.title')}</h3>
        <StatusPill status={status} />
      </div>
      <code className="block break-all font-mono text-base sm:text-lg">
        {addr.display}
      </code>
      <div className="flex flex-wrap items-center gap-1.5">
        <SecurityChip security={resolveSecurity(addr, probe)} />
        <ScopeChip scope={addr.scope} />
      </div>
      {probe && !checking ? <ProbeNote probe={probe} /> : null}
      {status === 'down' ? (
        <div className="space-y-2 rounded-md border border-border bg-background/60 p-3">
          <p className="text-sm">{t('hero.fixTitle')}</p>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={onAllow}>
              {t('actions.openOnNetwork')}
            </Button>
            <Link
              to="/apps/$name/network"
              params={{ name: appName }}
              className={buttonVariants({ size: 'sm', variant: 'outline' })}
            >
              {t('actions.addDomain')}
            </Link>
          </div>
          <p className="text-xs text-muted-foreground">{t('hero.fixHint')}</p>
        </div>
      ) : null}
      {!running ? (
        <p className="text-xs text-muted-foreground">{t('hero.stopped')}</p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <a
          href={addr.href}
          target="_blank"
          rel="noreferrer noopener"
          className={buttonVariants({ variant: 'default' })}
        >
          <ExternalLinkGlassIcon className="size-4.5" aria-hidden="true" />
          {t('actions.openApp')}
        </a>
        <Button variant="outline" onClick={() => copy(addr.value)}>
          {copied ? (
            <CheckGlassIcon className="size-4.5" aria-hidden="true" />
          ) : (
            <CopyGlassIcon className="size-4.5" aria-hidden="true" />
          )}
          {copied ? t('actions.copied') : t('actions.copyUrl')}
        </Button>
        {running ? (
          <Button variant="ghost" disabled={checking} onClick={onRecheck}>
            {t('actions.recheck')}
          </Button>
        ) : null}
      </div>
    </section>
  )
}
