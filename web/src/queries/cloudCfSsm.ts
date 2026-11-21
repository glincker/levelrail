import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'

export interface AwsCfDistribution {
  id: string
  status: string
  domain: string
  comment: string
  enabled: boolean
  aliases: string[]
}

export interface AwsCfInvalidation {
  id: string
  status: string
  created_at: string
}

export interface AwsSsmInstance {
  id: string
  ping_status: string
  platform: string
  platform_name: string
  agent_version: string
  computer_name: string
  last_ping: string
}

export interface AwsSsmInvocation {
  status: string
  detail: string
  exit_code: number
  stdout: string
  stderr: string
}

export interface AwsSsmSession {
  session_id: string
  token: string
  stream_url: string
  region: string
}

const enc = encodeURIComponent
const base = (id: string) => `/api/v1/cloud-accounts/${enc(id)}/aws`
const key = (id: string) =>
  [...cloudAccountKeys.all, 'aws', id, 'cfssm'] as const

export function useAwsCfDistributions(id: string) {
  return useQuery({
    queryKey: [...key(id), 'distributions'],
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ distributions: AwsCfDistribution[] | null }>(
          `${base(id)}/cloudfront/distributions`,
          { fallbackMessage: 'load distributions failed' },
        )
      ).distributions ?? [],
  })
}

export function useAwsCfInvalidations(id: string, did: string | null) {
  return useQuery({
    queryKey: [...key(id), 'invalidations', did ?? ''],
    enabled: did !== null,
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ invalidations: AwsCfInvalidation[] | null }>(
          `${base(id)}/cloudfront/distributions/${enc(did ?? '')}/invalidations`,
          { fallbackMessage: 'load invalidations failed' },
        )
      ).invalidations ?? [],
  })
}

export function useAwsCfInvalidate(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: ({ did, paths }: { did: string; paths: string[] }) =>
      apiRequest<AwsCfInvalidation>(
        `${base(id)}/cloudfront/distributions/${enc(did)}/invalidations`,
        {
          method: 'POST',
          body: { paths },
          fallbackMessage: 'invalidation failed',
        },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: key(id) }),
  })
}

export function useAwsSsmInstances(id: string) {
  return useQuery({
    queryKey: [...key(id), 'ssm-instances'],
    retry: false,
    queryFn: () =>
      apiRequest<{ region: string; instances: AwsSsmInstance[] | null }>(
        `${base(id)}/ssm/instances`,
        { fallbackMessage: 'load managed instances failed' },
      ),
  })
}

export function useAwsSsmRun(id: string) {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: async (v: {
      region: string
      instance: string
      command: string
    }) => {
      const sent = await apiRequest<{ command_id: string }>(
        `${base(id)}/ssm/commands`,
        { method: 'POST', body: v, fallbackMessage: 'run command failed' },
      )
      return {
        commandId: sent.command_id,
        invocation: await fetchSsmInvocation(id, sent.command_id, v),
      }
    },
  })
}

function fetchSsmInvocation(
  id: string,
  cid: string,
  v: { region: string; instance: string },
) {
  return apiRequest<AwsSsmInvocation>(
    `${base(id)}/ssm/commands/${enc(cid)}/invocation`,
    {
      method: 'POST',
      body: { region: v.region, instance: v.instance },
      fallbackMessage: 'load command result failed',
    },
  )
}

export function useAwsSsmRefresh(id: string) {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (v: { cid: string; region: string; instance: string }) =>
      fetchSsmInvocation(id, v.cid, v),
  })
}

export function useAwsSsmSession(id: string) {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (v: { region: string; instance: string }) =>
      apiRequest<AwsSsmSession>(`${base(id)}/ssm/sessions`, {
        method: 'POST',
        body: v,
        fallbackMessage: 'start session failed',
      }),
  })
}
