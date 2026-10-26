// Fetchers for the read-only stack template catalog (internal/api/stack_templates.go).
// Render never applies anything; it validates and returns files.

import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  StackTemplateDetail,
  StackTemplateRender,
  StackTemplateSummary,
} from '../types/stackTemplates'

export const stackTemplateKeys = {
  all: ['stack-templates'] as const,
  list: () => [...stackTemplateKeys.all, 'list'] as const,
  detail: (id: string) => [...stackTemplateKeys.all, 'detail', id] as const,
  render: (id: string, variables: Record<string, string>) =>
    [...stackTemplateKeys.all, 'render', id, variables] as const,
}

const base = (id: string) => `/api/v1/stack-templates/${encodeURIComponent(id)}`

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
  )
}

export function stackTemplatesQueryOptions() {
  return queryOptions({
    queryKey: stackTemplateKeys.list(),
    queryFn: async (): Promise<StackTemplateSummary[]> => {
      const res = await fetch('/api/v1/stack-templates')
      if (!res.ok) throw await failure(res, 'list stack templates')
      const body = (await res.json()) as { templates: StackTemplateSummary[] }
      return body.templates
    },
  })
}

export function stackTemplateQueryOptions(id: string) {
  return queryOptions({
    queryKey: stackTemplateKeys.detail(id),
    queryFn: async (): Promise<StackTemplateDetail> => {
      const res = await fetch(base(id))
      if (!res.ok) throw await failure(res, 'load stack template')
      return (await res.json()) as StackTemplateDetail
    },
  })
}

// A 422 still carries the rendered files plus the validation issues, so it is data, not an error.
export function stackTemplateRenderQueryOptions(
  id: string,
  variables: Record<string, string>,
) {
  return queryOptions({
    queryKey: stackTemplateKeys.render(id, variables),
    queryFn: async (): Promise<StackTemplateRender> => {
      const res = await fetch(`${base(id)}/render`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ variables }),
      })
      if (!res.ok && res.status !== 422) {
        throw await failure(res, 'render stack template')
      }
      return (await res.json()) as StackTemplateRender
    },
  })
}
