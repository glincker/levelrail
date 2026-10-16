import { useTranslation } from 'react-i18next'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  CheckIcon,
  CircleDashedIcon,
  CopyIcon,
  SpinnerGapIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
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
    live: { variant: 'success', Icon: CheckCircleIcon },
    down: { variant: 'warning', Icon: WarningIcon },
    unchecked: { variant: 'muted', Icon: CircleDashedIcon },
    checking: { variant: 'muted', Icon: SpinnerGapIcon },
    stopped: { variant: 'muted', Icon: CircleDashedIcon },
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
  running: boolean
  probe?: ProbeTarget
  checking: boolean
  onRecheck: () => void
}

/** The best address to open, with its live status and TLS state. */
export function ReachHero({
  addr,
  running,
  probe,
  checking,
  onRecheck,
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
          <ArrowSquareOutIcon className="size-4" aria-hidden="true" />
          {t('actions.openApp')}
        </a>
        <Button variant="outline" onClick={() => copy(addr.value)}>
          {copied ? (
            <CheckIcon className="size-4" aria-hidden="true" />
          ) : (
            <CopyIcon className="size-4" aria-hidden="true" />
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
