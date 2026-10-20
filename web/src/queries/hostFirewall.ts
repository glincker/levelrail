import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { HostFirewall } from '../types/hostFirewall'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { systemDoctorKeys } from './systemDoctor'

export const hostFirewallKeys = {
  all: ['host-firewall'] as const,
}

export async function fetchHostFirewall(): Promise<HostFirewall> {
  const res = await fetch('/api/v1/firewall/host')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch host firewall failed: ${res.status}`),
    )
  }
  return (await res.json()) as HostFirewall
}

export function useHostFirewall() {
  return useQuery({
    queryKey: hostFirewallKeys.all,
    queryFn: fetchHostFirewall,
    staleTime: 10_000,
  })
}

export interface SetHostFirewallInput {
  enable: boolean
  dryRun?: boolean
}

export async function setHostFirewall({
  enable,
  dryRun = false,
}: SetHostFirewallInput): Promise<HostFirewall> {
  const res = await fetch(
    `/api/v1/firewall/host/${enable ? 'enable' : 'disable'}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dry_run: dryRun }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set host firewall failed: ${res.status}`),
    )
  }
  return (await res.json()) as HostFirewall
}

export function useSetHostFirewall() {
  const queryClient = useQueryClient()
  return useMutation<HostFirewall, ApiError, SetHostFirewallInput>({
    mutationFn: setHostFirewall,
    onSuccess: (_data, input) => {
      if (input.dryRun) return
      void queryClient.invalidateQueries({ queryKey: hostFirewallKeys.all })
      void queryClient.invalidateQueries({ queryKey: systemDoctorKeys.all })
    },
  })
}
