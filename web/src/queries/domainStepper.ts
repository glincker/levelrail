// Polling for the domain stepper's live DNS check. The cap and interval are
// env knobs so a slow-propagating registrar can be tuned without a rebuild.
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { domainCheckKeys, fetchDomainCheck } from './domainCheck'

function envPositive(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

export const STEPPER_POLL_MS = envPositive(
  import.meta.env.VITE_DOMAIN_CHECK_POLL_MS,
  5_000,
)
export const STEPPER_MAX_POLLS = envPositive(
  import.meta.env.VITE_DOMAIN_CHECK_MAX_POLLS,
  24,
)

export function useStepperDomainCheck(
  appName: string,
  domain: string,
  enabled: boolean,
) {
  // Re-check restarts the poll budget by moving the baseline up.
  const qc = useQueryClient()
  const key = domainCheckKeys.detail(appName, domain)
  const [baseline, setBaseline] = useState(0)
  const query = useQuery({
    queryKey: key,
    queryFn: () => fetchDomainCheck(appName, domain),
    enabled: enabled && domain.length > 0,
    refetchInterval: (q) => {
      if (q.state.data?.status === 'connected') return false
      if (q.state.dataUpdateCount - baseline >= STEPPER_MAX_POLLS) return false
      return STEPPER_POLL_MS
    },
  })
  const capped =
    query.data?.status !== 'connected' &&
    (qc.getQueryState(key)?.dataUpdateCount ?? 0) - baseline >=
      STEPPER_MAX_POLLS
  function recheck() {
    setBaseline(qc.getQueryState(key)?.dataUpdateCount ?? 0)
    void query.refetch()
  }
  return { ...query, capped, recheck }
}
