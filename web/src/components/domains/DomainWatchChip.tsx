import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { secondsUntil } from '../../lib/watchTime'
import { useDomainWatch, type WatchEntry } from '../../queries/domainsWatch'

type Variant = VariantProps<typeof badgeVariants>['variant']

const VARIANT: Record<Exclude<WatchEntry['state'], 'live'>, Variant> = {
  waiting_dns: 'warning',
  propagating: 'warning',
  waiting_certificate: 'info',
  stalled: 'destructive',
}

export function WatchChipView({
  entry,
  now,
}: {
  entry: WatchEntry
  now: number
}) {
  const { t } = useTranslation('domains')
  if (entry.state === 'live') return null
  const secs = entry.stopped ? null : secondsUntil(entry.next_check, now)
  const next =
    secs === null
      ? null
      : secs < 60
        ? t('watch.nextInSeconds', { count: secs })
        : t('watch.nextInMinutes', { count: Math.ceil(secs / 60) })
  return (
    <Badge variant={VARIANT[entry.state]} className="gap-1" role="status">
      <span>{t(`watch.state.${entry.state}`)}</span>
      {next ? <span className="font-normal opacity-80">{next}</span> : null}
    </Badge>
  )
}

// Hidden unless the server watches this hostname and it is not live yet.
export function DomainWatchChip({ domain }: { domain: string }) {
  const entry = useDomainWatch(domain)
  const [now, setNow] = useState(() => Date.now())
  const ticking = entry !== undefined && entry.state !== 'live'
  useEffect(() => {
    if (!ticking) return
    const id = setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      clearInterval(id)
    }
  }, [ticking])
  if (!entry) return null
  return <WatchChipView entry={entry} now={now} />
}
