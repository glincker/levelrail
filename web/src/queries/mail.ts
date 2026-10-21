// /api/v1/mail/*: templates, audiences, suppressions and broadcasts
// (internal/api/mail_*_api.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface MailToken {
  name: string
  description: string
  sample: string
  scope: 'global' | 'recipient' | 'event'
}

export interface MailTemplateBody {
  subject: string
  preheader: string
  body: string
}

export interface MailTemplate {
  key: string
  label: string
  description: string
  category: 'system' | 'announcement'
  tokens: string[]
  default: MailTemplateBody
  current: MailTemplateBody
  customized: boolean
  updated_at?: string
}

export interface MailLayout {
  header_image_url: string
  footer_text: string
}

export interface MailTemplatesResponse {
  templates: MailTemplate[]
  layout: MailLayout
}

export interface MailVersion extends MailTemplateBody {
  event_key: string
  version: number
  created_by: string
  created_at: string
}

export interface MailPreview {
  subject: string
  preheader: string
  text: string
  html: string
}

export interface MailPreviewDraft extends Partial<MailTemplateBody> {
  header_image_url?: string
  footer_text?: string
  scheme?: 'light' | 'dark'
}

export interface MailRules {
  all_users: boolean
  roles: string[]
  teams: string[]
  projects: string[]
  email_domains: string[]
  inactive_days: number
  extra_addresses: string[]
  exclude_addresses: string[]
}

export interface MailGroup {
  id: string
  name: string
  description: string
  rules: MailRules
  recipients: number
  suppressed: number
}

export interface MailAudiencePreview {
  total: number
  deliverable: number
  suppressed: number
  sample: string[]
}

export interface MailAudienceOptions {
  roles: string[]
  teams: { id: string; name: string }[]
  projects: { id: string; name: string; team_id?: string }[]
}

export interface MailSuppression {
  email: string
  reason: string
  created_at: string
}

export interface MailBroadcastInput {
  subject: string
  body: string
  template_key?: string
  audience?: string
  recipients?: string[]
  schedule_at?: string
  dry_run?: boolean
  expected_recipients?: number
}

export interface MailBroadcastPlan extends MailAudiencePreview {
  cap_remaining: number
  provider_configured: boolean
}

export type MailBroadcastStatus =
  'scheduled' | 'sending' | 'completed' | 'cancelled' | 'failed'

export interface MailBroadcast {
  id: string
  subject: string
  body: string
  status: MailBroadcastStatus
  scheduled_at: string
  total: number
  sent: number
  failed: number
  suppressed: number
  error: string
  created_by: string
  created_at: string
  finished_at: string
}

export interface MailRecipient {
  email: string
  status: string
  error: string
  sent_at: string
}

export class MailApiError extends ApiError {
  readonly code: string

  constructor(status: number, message: string, code: string) {
    super(status, message)
    this.code = code
  }
}

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const clone = res.clone()
    const raw = (await clone.json().catch(() => null)) as {
      code?: string
    } | null
    throw new MailApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
      raw?.code ?? '',
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const mailKeys = {
  tokens: ['mail', 'tokens'] as const,
  templates: ['mail', 'templates'] as const,
  versions: (key: string) => ['mail', 'versions', key] as const,
  groups: ['mail', 'groups'] as const,
  options: ['mail', 'options'] as const,
  suppressions: ['mail', 'suppressions'] as const,
  broadcasts: ['mail', 'broadcasts'] as const,
  broadcast: (id: string) => ['mail', 'broadcast', id] as const,
}

export function mailTokensQueryOptions() {
  return queryOptions({
    queryKey: mailKeys.tokens,
    queryFn: async () =>
      (await call<{ tokens: MailToken[] }>('GET', '/api/v1/mail/tokens'))
        .tokens,
    staleTime: 5 * 60_000,
  })
}

export function mailTemplatesQueryOptions() {
  return queryOptions({
    queryKey: mailKeys.templates,
    queryFn: () => call<MailTemplatesResponse>('GET', '/api/v1/mail/templates'),
    staleTime: 30_000,
  })
}

export function useMailTokens() {
  return useQuery(mailTokensQueryOptions())
}

export function useMailTemplates() {
  return useQuery(mailTemplatesQueryOptions())
}

export function useMailVersions(key: string) {
  return useQuery({
    queryKey: mailKeys.versions(key),
    queryFn: async () =>
      (
        await call<{ versions: MailVersion[] }>(
          'GET',
          `/api/v1/mail/templates/${encodeURIComponent(key)}/versions`,
        )
      ).versions,
  })
}

export function useSaveMailTemplate(key: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: MailTemplateBody) =>
      call<MailTemplate>(
        'PUT',
        `/api/v1/mail/templates/${encodeURIComponent(key)}`,
        body,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.templates })
      void qc.invalidateQueries({ queryKey: mailKeys.versions(key) })
    },
  })
}

export function useResetMailTemplate(key: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      call<MailTemplate>(
        'DELETE',
        `/api/v1/mail/templates/${encodeURIComponent(key)}`,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.templates })
    },
  })
}

export function useRestoreMailVersion(key: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (version: number) =>
      call<MailTemplate>(
        'POST',
        `/api/v1/mail/templates/${encodeURIComponent(key)}/versions/${version}/restore`,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.templates })
      void qc.invalidateQueries({ queryKey: mailKeys.versions(key) })
    },
  })
}

export function useSaveMailLayout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (layout: MailLayout) =>
      call<MailLayout>('PUT', '/api/v1/mail/layout', layout),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.templates })
    },
  })
}

export function previewMailTemplate(
  key: string,
  draft: MailPreviewDraft,
): Promise<MailPreview> {
  return call<MailPreview>(
    'POST',
    `/api/v1/mail/templates/${encodeURIComponent(key)}/preview`,
    draft,
  )
}

export function useSendMailTest(key: string) {
  return useMutation({
    mutationFn: () =>
      call<{ sent_to: string }>(
        'POST',
        `/api/v1/mail/templates/${encodeURIComponent(key)}/test`,
        {},
      ),
  })
}

export function useMailGroups() {
  return useQuery({
    queryKey: mailKeys.groups,
    queryFn: async () =>
      (await call<{ groups: MailGroup[] }>('GET', '/api/v1/mail/groups'))
        .groups,
  })
}

export function useMailAudienceOptions() {
  return useQuery({
    queryKey: mailKeys.options,
    queryFn: () =>
      call<MailAudienceOptions>('GET', '/api/v1/mail/audience-options'),
    staleTime: 60_000,
  })
}

export function previewMailAudience(
  rules: MailRules,
): Promise<MailAudiencePreview> {
  return call<MailAudiencePreview>('POST', '/api/v1/mail/audience/preview', {
    name: 'preview',
    rules,
  })
}

export interface MailGroupInput {
  name: string
  description: string
  rules: MailRules
}

export function useSaveMailGroup(id?: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: MailGroupInput) =>
      id
        ? call<MailGroup>(
            'PUT',
            `/api/v1/mail/groups/${encodeURIComponent(id)}`,
            input,
          )
        : call<MailGroup>('POST', '/api/v1/mail/groups', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.groups })
    },
  })
}

export function useDeleteMailGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      call<void>('DELETE', `/api/v1/mail/groups/${encodeURIComponent(id)}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.groups })
    },
  })
}

export function useMailSuppressions() {
  return useQuery({
    queryKey: mailKeys.suppressions,
    queryFn: async () =>
      (
        await call<{ suppressions: MailSuppression[] }>(
          'GET',
          '/api/v1/mail/suppressions',
        )
      ).suppressions,
  })
}

export function useAddMailSuppression() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (email: string) =>
      call<MailSuppression>('POST', '/api/v1/mail/suppressions', { email }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.suppressions })
      void qc.invalidateQueries({ queryKey: mailKeys.groups })
    },
  })
}

export function useRemoveMailSuppression() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (email: string) =>
      call<void>(
        'DELETE',
        `/api/v1/mail/suppressions/${encodeURIComponent(email)}`,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.suppressions })
      void qc.invalidateQueries({ queryKey: mailKeys.groups })
    },
  })
}

export function planMailBroadcast(
  input: MailBroadcastInput,
): Promise<MailBroadcastPlan> {
  return call<MailBroadcastPlan>('POST', '/api/v1/mail/broadcasts', {
    ...input,
    dry_run: true,
  })
}

export function useSendMailBroadcast() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: MailBroadcastInput) =>
      call<MailBroadcast>('POST', '/api/v1/mail/broadcasts', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.broadcasts })
    },
  })
}

export function useSendMailBroadcastTest() {
  return useMutation({
    mutationFn: (draft: { subject: string; body: string }) =>
      call<{ sent_to: string }>('POST', '/api/v1/mail/broadcasts/test', draft),
  })
}

export function useMailBroadcasts() {
  return useQuery({
    queryKey: mailKeys.broadcasts,
    queryFn: async () =>
      (
        await call<{ broadcasts: MailBroadcast[] }>(
          'GET',
          '/api/v1/mail/broadcasts',
        )
      ).broadcasts,
    refetchInterval: (query) =>
      query.state.data?.some(
        (b) => b.status === 'sending' || b.status === 'scheduled',
      )
        ? 3000
        : false,
  })
}

export function useMailBroadcast(id: string | null) {
  return useQuery({
    queryKey: mailKeys.broadcast(id ?? ''),
    enabled: id !== null,
    queryFn: () =>
      call<{ broadcast: MailBroadcast; recipients: MailRecipient[] }>(
        'GET',
        `/api/v1/mail/broadcasts/${encodeURIComponent(id ?? '')}`,
      ),
    refetchInterval: (query) =>
      query.state.data?.broadcast.status === 'sending' ? 2000 : false,
  })
}

export function useCancelMailBroadcast() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      call<MailBroadcast>(
        'POST',
        `/api/v1/mail/broadcasts/${encodeURIComponent(id)}/cancel`,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mailKeys.broadcasts })
    },
  })
}
