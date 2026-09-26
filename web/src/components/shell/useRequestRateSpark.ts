import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { requestSeriesQueryOptions } from '../../queries/requests'

const WINDOW_MS = 60 * 60 * 1000
const MINUTE_MS = 60 * 1000

export function useRequestRateSpark(name: string): number[] {
  const [range] = React.useState(() => {
    const to = new Date(Math.floor(Date.now() / MINUTE_MS) * MINUTE_MS)
    return { from: new Date(to.getTime() - WINDOW_MS), to, step: '2m' }
  })
  const query = useQuery({
    ...requestSeriesQueryOptions(name, range),
    retry: false,
    staleTime: 60_000,
  })
  return (query.data?.points ?? []).map((p) => p.rate_per_sec)
}
