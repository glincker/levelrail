// /api/v1/apps/{name}/forms (internal/api/forms_admin.go, forms_submissions.go):
// form definitions, their inbox, spam flags and CSV export.

import {
  infiniteQueryOptions,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { FormSchema } from '../lib/formSnippet'

export interface AppForm {
  id: string
  app: string
  name: string
  redirect_url: string
  honeypot_field: string
  notify_channel_ids: string[]
  endpoint_path: string
  endpoint: string
  snippet: string
  snippets: { html: string; fetch: string; minimal: string }
  schema: FormSchema
  /** "local" means no public URL is configured and the endpoint only works from this machine. */
  endpoint_source: 'domain' | 'public_url' | 'local'
  submissions: number
  spam_submissions: number
  created_at: string
}

export interface FormsList {
  forms: AppForm[]
  retention_days: number
}

export interface FormSubmission {
  id: number
  fields: Record<string, string>
  spam: boolean
  test?: boolean
  spam_reason?: string
  created_at: string
}

export interface FormTestResult {
  submission_id: number
  spam: boolean
  fields: Record<string, string>
}

export interface FormSubmissionsPage {
  submissions: FormSubmission[]
  next_before: number
}

export type SpamFilter = 'all' | 'inbox' | 'spam'

export interface FormSettingsInput {
  redirect_url: string
  honeypot_field: string
  notify_channel_ids: string[]
}

const PAGE_SIZE = 50

export const formKeys = {
  list: (app: string) => [...appKeys.detail(app), 'forms'] as const,
  inbox: (app: string, form: string, q: string, spam: SpamFilter) =>
    [...appKeys.detail(app), 'forms', form, 'inbox', q, spam] as const,
}

function formsBase(app: string, form?: string): string {
  const base = `/api/v1/apps/${encodeURIComponent(app)}/forms`
  return form ? `${base}/${encodeURIComponent(form)}` : base
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function spamParam(filter: SpamFilter): string | undefined {
  if (filter === 'spam') return 'true'
  if (filter === 'inbox') return 'false'
  return undefined
}

export function submissionsQuery(
  q: string,
  spam: SpamFilter,
  before?: number,
  limit = PAGE_SIZE,
): string {
  const p = new URLSearchParams()
  if (q.trim()) p.set('q', q.trim())
  const s = spamParam(spam)
  if (s) p.set('spam', s)
  if (before) p.set('before', String(before))
  p.set('limit', String(limit))
  return p.toString()
}

export function formsQueryOptions(app: string) {
  return queryOptions({
    queryKey: formKeys.list(app),
    queryFn: async (): Promise<FormsList> => {
      const res = await fetch(formsBase(app))
      if (!res.ok) throw await failure(res, 'fetch forms')
      return (await res.json()) as FormsList
    },
  })
}

export function useAppForms(app: string) {
  return useQuery(formsQueryOptions(app))
}

export function inboxQueryOptions(
  app: string,
  form: string,
  q: string,
  spam: SpamFilter,
) {
  return infiniteQueryOptions({
    queryKey: formKeys.inbox(app, form, q, spam),
    initialPageParam: 0,
    queryFn: async ({ pageParam }): Promise<FormSubmissionsPage> => {
      const res = await fetch(
        `${formsBase(app, form)}/submissions?${submissionsQuery(q, spam, pageParam)}`,
      )
      if (!res.ok) throw await failure(res, 'fetch submissions')
      return (await res.json()) as FormSubmissionsPage
    },
    getNextPageParam: (last) =>
      last.next_before > 0 ? last.next_before : undefined,
  })
}

export function useFormInbox(
  app: string,
  form: string,
  q: string,
  spam: SpamFilter,
) {
  return useInfiniteQuery(inboxQueryOptions(app, form, q, spam))
}

async function send(
  url: string,
  method: string,
  body?: unknown,
): Promise<Response> {
  const res = await fetch(url, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) throw await failure(res, 'forms request')
  return res
}

export function useFormActions(app: string) {
  const qc = useQueryClient()
  const refresh = () =>
    qc.invalidateQueries({ queryKey: [...appKeys.detail(app), 'forms'] })
  const create = useMutation<AppForm, ApiError, string>({
    mutationFn: async (name) =>
      (await (await send(formsBase(app), 'POST', { name })).json()) as AppForm,
    onSuccess: refresh,
  })
  const save = useMutation<
    AppForm,
    ApiError,
    { form: string; input: FormSettingsInput }
  >({
    mutationFn: async ({ form, input }) =>
      (await (
        await send(formsBase(app, form), 'PUT', input)
      ).json()) as AppForm,
    onSuccess: refresh,
  })
  const remove = useMutation<void, ApiError, string>({
    mutationFn: async (form) => {
      await send(formsBase(app, form), 'DELETE')
    },
    onSuccess: refresh,
  })
  const setSpam = useMutation<
    void,
    ApiError,
    { form: string; id: number; spam: boolean }
  >({
    mutationFn: async ({ form, id, spam }) => {
      await send(`${formsBase(app, form)}/submissions/${id}/spam`, 'PUT', {
        spam,
      })
    },
    onSuccess: refresh,
  })
  const removeSubmission = useMutation<
    void,
    ApiError,
    { form: string; id: number }
  >({
    mutationFn: async ({ form, id }) => {
      await send(`${formsBase(app, form)}/submissions/${id}`, 'DELETE')
    },
    onSuccess: refresh,
  })
  const saveFields = useMutation<
    AppForm,
    ApiError,
    { form: string; schema: FormSchema }
  >({
    mutationFn: async ({ form, schema }) =>
      (await (
        await send(`${formsBase(app, form)}/fields`, 'PUT', schema)
      ).json()) as AppForm,
    onSuccess: refresh,
  })
  const sendTest = useMutation<FormTestResult, ApiError, string>({
    mutationFn: async (form) =>
      (await (
        await send(`${formsBase(app, form)}/test`, 'POST')
      ).json()) as FormTestResult,
    onSuccess: refresh,
  })
  return {
    create,
    save,
    saveFields,
    sendTest,
    remove,
    setSpam,
    removeSubmission,
  }
}

export function exportUrl(
  app: string,
  form: string,
  q: string,
  spam: SpamFilter,
): string {
  const p = new URLSearchParams()
  if (q.trim()) p.set('q', q.trim())
  const s = spamParam(spam)
  if (s) p.set('spam', s)
  const qs = p.toString()
  return `${formsBase(app, form)}/export${qs ? `?${qs}` : ''}`
}
