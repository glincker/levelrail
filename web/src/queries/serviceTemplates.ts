// Query-key factory and fetchers for GET /api/v1/service-templates and
// GET /api/v1/service-templates/{id} (internal/api/service_templates.go):
// Levelrail's own curated one-click catalog (internal/catalog, ADR 015).
// The list endpoint omits each template's compose body on purpose (cheap
// for a grid); the detail endpoint adds it back in, fetched only once an
// operator picks a specific template.

import {
  useMutation,
  useQuery,
  useQueryClient,
  queryOptions,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import type { ComposeDeployResponse } from './compose'

export const serviceTemplateKeys = {
  all: ['service-templates'] as const,
  list: () => [...serviceTemplateKeys.all, 'list'] as const,
  detail: (id: string) => [...serviceTemplateKeys.all, 'detail', id] as const,
}

// Mirrors serviceTemplateListItem's wire shape exactly.
export interface ServiceTemplateListItem {
  id: string
  name: string
  slogan: string
  category: string
  documentation_url: string
  // Static pre-deploy advisory, not checked against any node's real
  // available memory (internal/catalog.Template's own doc comment on
  // why). Absent (omitempty) when a template has no advisory.
  recommended_memory_bytes?: number
  requires_gpu?: boolean
  // True when the template's Compose body has a required secret with
  // no default (internal/api/service_templates.go's
  // Router.templateRequiresConfig): the one-click deploy path
  // (useDeployServiceTemplateNow below) isn't safe for these, the UI
  // falls back to the pre-filled wizard step instead.
  requires_configuration: boolean
}

// Mirrors serviceTemplateDetail's wire shape: the list item's fields plus
// the full compose.yaml body.
export interface ServiceTemplateDetail extends ServiceTemplateListItem {
  compose: string
}

export async function fetchServiceTemplates(): Promise<
  ServiceTemplateListItem[]
> {
  const res = await fetch('/api/v1/service-templates')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch service templates failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ServiceTemplateListItem[]
}

export async function fetchServiceTemplate(
  id: string,
): Promise<ServiceTemplateDetail> {
  const res = await fetch(`/api/v1/service-templates/${encodeURIComponent(id)}`)
  if (res.status === 404) {
    throw new ApiError(404, `service template not found: ${id}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch service template failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ServiceTemplateDetail
}

// Same shared-options pattern appListQueryOptions/appDetailQueryOptions
// establish (queries/apps.ts): one definition a route loader and a
// component's useQuery call can both share. staleTime matches
// databaseEnginesQueryOptions' own reasoning: this catalog only changes
// when the control plane itself is upgraded, not during a normal session.
export function serviceTemplatesQueryOptions() {
  return queryOptions({
    queryKey: serviceTemplateKeys.list(),
    queryFn: fetchServiceTemplates,
    staleTime: 60_000,
  })
}

export function serviceTemplateQueryOptions(id: string) {
  return queryOptions({
    queryKey: serviceTemplateKeys.detail(id),
    queryFn: () => fetchServiceTemplate(id),
    staleTime: 60_000,
  })
}

export function useServiceTemplates() {
  return useQuery(serviceTemplatesQueryOptions())
}

// enabled: !!id so this stays idle until a template is actually picked,
// same reasoning useApp-style detail hooks already follow elsewhere.
export function useServiceTemplate(id: string) {
  return useQuery({
    ...serviceTemplateQueryOptions(id),
    enabled: !!id,
  })
}

// POST /api/v1/service-templates/{id}/deploy
// (internal/api/service_templates.go's handleDeployServiceTemplateNow):
// the one-click fast path for a template with no required, default-less
// secret. The backend picks the app name (a slugified, collision-
// checked id) and deploys with whatever defaults the template's own
// Compose body already declares, reusing the exact same
// deployComposeBody core useDeployCompose's endpoint calls, so the
// response is the same ComposeDeployResponse shape.
export async function deployServiceTemplateNow(
  id: string,
): Promise<ComposeDeployResponse> {
  const res = await fetch(
    `/api/v1/service-templates/${encodeURIComponent(id)}/deploy`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `deploy service template failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ComposeDeployResponse
}

// Same cache-write shape useDeployCompose already establishes: every
// returned service is a full AppDetail, written straight into its own
// detail cache entry.
export function useDeployServiceTemplateNow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deployServiceTemplateNow,
    onSuccess: (result) => {
      for (const service of result.services) {
        queryClient.setQueryData(appKeys.detail(service.name), service)
      }
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
