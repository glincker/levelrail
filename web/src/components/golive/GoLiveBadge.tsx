import { useQuery } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { liveBadgesQueryOptions } from '../../queries/goLiveChecklist'
import type { LiveState } from '../../queries/goLiveChecklist'
import { STATE_VARIANT } from './liveState'

function goLivePath(app: string, domain: string): string {
  const q = domain ? `?domain=${encodeURIComponent(domain)}` : ''
  return `/apps/${encodeURIComponent(app)}/go-live${q}`
}

/** LiveStateBadge shows one state as a badge that opens the checklist. */
export function LiveStateBadge({
  app,
  domain,
  state,
}: {
  app: string
  domain: string
  state: LiveState
}) {
  const { t } = useTranslation('golive')
  const router = useRouter()
  const href = goLivePath(app, domain)
  return (
    <a
      href={href}
      aria-label={t('badge.link', { domain })}
      onClick={(e) => {
        e.preventDefault()
        void router.navigate({ href })
      }}
    >
      <Badge variant={STATE_VARIANT[state]}>{t(`state.${state}`)}</Badge>
    </a>
  )
}

/** GoLiveBadge looks the domain up in the shared status query, so a list of rows costs one request. */
export function GoLiveBadge({ app, domain }: { app: string; domain: string }) {
  const { data } = useQuery(liveBadgesQueryOptions())
  const entry = data?.find((d) => d.app === app && d.domain === domain)
  if (!entry) return null
  return <LiveStateBadge app={app} domain={domain} state={entry.state} />
}

/** AppLiveBadge shows the state of the app's first domain, for the app overview. */
export function AppLiveBadge({ app }: { app: string }) {
  const { data } = useQuery(liveBadgesQueryOptions())
  const entry = data?.find((d) => d.app === app)
  if (!entry) return null
  return <LiveStateBadge app={app} domain={entry.domain} state={entry.state} />
}

/** GoLiveLink points from the domain diagnosis to the guided checklist. */
export function GoLiveLink({ href }: { href: string }) {
  const { t } = useTranslation('golive')
  const router = useRouter()
  return (
    <a
      href={href}
      className="text-sm text-primary underline underline-offset-2"
      onClick={(e) => {
        e.preventDefault()
        void router.navigate({ href })
      }}
    >
      {t('diagnoseLink')}
    </a>
  )
}
