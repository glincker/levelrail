// Query-key factory, fetchers, and mutations for operator-defined
// templates (internal/api/service_templates_custom.go). A custom
// template's own id still resolves and deploys through
// queries/serviceTemplates.ts's existing routes, so this file only
// covers what's new: saving, listing "your templates", and deleting.

import {
  useMutation,
  useQuery,
  useQueryClient,
  queryOptions,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { serviceTemplateKeys } from './serviceTemplates'

export const customTemplateKeys = {
  all: ['templates', 'custom'] as const,
  list: () => [...customTemplateKeys.all, 'list'] as const,
}

// Mirrors customTemplateListItem's wire shape exactly.
export interface CustomTemplateListItem {
  id: string
  name: string
  description: string
  source_app?: string
  // True when a captured secret or other placeholder is still
  // unresolved; the one-click deploy path isn't safe for these.
  requires_configuration: boolean
  created_at: string
}

// Mirrors customTemplateDetail: save-as-template's response, adding
// the compose body and which env keys ended up required.
export interface CustomTemplateDetail extends CustomTemplateListItem {
  compose: string
  required_env_keys?: string[]
}

export interface SaveAppAsTemplateRequest {
  name: string
  description?: string
}

export async function fetchCustomTemplates(): Promise<
  CustomTemplateListItem[]
> {
  const res = await fetch('/api/v1/templates/custom')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch custom templates failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CustomTemplateListItem[]
}

export async function saveAppAsTemplate(
  appName: string,
  req: SaveAppAsTemplateRequest,
): Promise<CustomTemplateDetail> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/save-as-template`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `save app as template failed: ${res.status}`),
    )
  }
  return (await res.json()) as CustomTemplateDetail
}

export async function deleteCustomTemplate(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/templates/custom/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok && res.status !== 404) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `delete custom template failed: ${res.status}`,
      ),
    )
  }
}

export function customTemplatesQueryOptions() {
  return queryOptions({
    queryKey: customTemplateKeys.list(),
    queryFn: fetchCustomTemplates,
    staleTime: 30_000,
  })
}

export function useCustomTemplates() {
  return useQuery(customTemplatesQueryOptions())
}

// Invalidates the list so the new template shows up under "Your
// templates"; the built-in catalog's own cache is untouched.
export function useSaveAppAsTemplate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      appName,
      request,
    }: {
      appName: string
      request: SaveAppAsTemplateRequest
    }) => saveAppAsTemplate(appName, request),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: customTemplateKeys.list(),
      })
    },
  })
}

export function useDeleteCustomTemplate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCustomTemplate(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({
        queryKey: customTemplateKeys.list(),
      })
      // GET /api/v1/service-templates/{id} (resolveTemplate) would now
      // 404 for this id; drop its cached detail so a stale "preview"
      // panel can't keep showing it.
      queryClient.removeQueries({ queryKey: serviceTemplateKeys.detail(id) })
    },
  })
}
