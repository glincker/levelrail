import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ingressDomainCheckQueryOptions } from '../../queries/domains'
import { certificatesQueryOptions } from '../../queries/certificates'
import { gitProvidersQueryOptions } from '../../queries/gitProviders'
import { appListQueryOptions } from '../../queries/apps'
import {
  domainProgress,
  firstAppPhase,
  pickTrackedApp,
  pollInterval,
} from '../../lib/setupWizard'
import {
  APP_POLL_BUDGET_MS,
  APP_POLL_INTERVAL_MS,
  APP_SLOW_AFTER_MS,
  CERT_POLL_INTERVAL_MS,
  DNS_POLL_INTERVAL_MS,
  DOMAIN_POLL_BUDGET_MS,
  GIT_POLL_BUDGET_MS,
  GIT_POLL_INTERVAL_MS,
  SAMPLE_APP,
} from './types'

const CLOCK_TICK_MS = 5_000

/** useNow re-renders on a fixed tick so elapsed-time states advance even when polled data does not change. */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), CLOCK_TICK_MS)
    return () => clearInterval(id)
  }, [])
  return now
}

/** useDomainVerification polls DNS, then certificate issuance, for a saved dashboard domain. */
export function useDomainVerification(
  domain: string,
  saved: boolean,
  startedAt: number,
  publicIp: string | undefined,
) {
  const check = useQuery({
    ...ingressDomainCheckQueryOptions(),
    enabled: saved,
    refetchInterval: (q) => {
      const data = q.state.data
      const done =
        data?.configured === true &&
        data.domain === domain &&
        data.status === 'connected'
      return pollInterval(
        done,
        startedAt,
        Date.now(),
        DNS_POLL_INTERVAL_MS,
        DOMAIN_POLL_BUDGET_MS,
      )
    },
  })
  const dnsOk =
    saved &&
    domainProgress({
      domain,
      check: check.data,
      certificates: undefined,
      publicIp,
    }).dns === 'ok'
  const certs = useQuery({
    ...certificatesQueryOptions(),
    enabled: dnsOk,
    refetchInterval: (q) => {
      const found = (q.state.data ?? []).some(
        (c) => c.domain === domain || (c.sans ?? []).includes(domain),
      )
      return pollInterval(
        found,
        startedAt,
        Date.now(),
        CERT_POLL_INTERVAL_MS,
        DOMAIN_POLL_BUDGET_MS,
      )
    },
  })
  const progress = saved
    ? domainProgress({
        domain,
        check: check.data,
        certificates: certs.data,
        publicIp,
      })
    : undefined
  const budgetSpent = useNow() - startedAt >= DOMAIN_POLL_BUDGET_MS
  return {
    progress,
    budgetSpent,
    refetch: () => Promise.all([check.refetch(), certs.refetch()]),
  }
}

/** useGitProviderPolling refetches provider status until one connects, including on tab focus. */
export function useGitProviderPolling(startedAt: number) {
  return useQuery({
    ...gitProvidersQueryOptions(),
    refetchOnWindowFocus: 'always',
    refetchInterval: (q) => {
      const connected = (q.state.data ?? []).some((p) => p.connected)
      return pollInterval(
        connected,
        startedAt,
        Date.now(),
        GIT_POLL_INTERVAL_MS,
        GIT_POLL_BUDGET_MS,
      )
    },
  })
}

/** useFirstAppPolling tracks the first app until it is healthy or the poll budget runs out. */
export function useFirstAppPolling(trackingSince: number) {
  const apps = useQuery({
    ...appListQueryOptions(),
    refetchInterval: (q) => {
      const app = pickTrackedApp(q.state.data ?? [], SAMPLE_APP.name)
      const live =
        firstAppPhase(app, trackingSince, Date.now(), APP_SLOW_AFTER_MS) ===
        'live'
      return pollInterval(
        live,
        trackingSince,
        Date.now(),
        APP_POLL_INTERVAL_MS,
        APP_POLL_BUDGET_MS,
      )
    },
  })
  const now = useNow()
  const app = pickTrackedApp(apps.data ?? [], SAMPLE_APP.name)
  const phase = firstAppPhase(app, trackingSince, now, APP_SLOW_AFTER_MS)
  return { app, phase, isLoading: apps.isLoading }
}
