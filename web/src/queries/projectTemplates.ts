import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/projecttpl's Info, Param, File and FieldError.
export interface ProjectTemplateParam {
  name: string
  label: string
  kind: 'string' | 'int' | 'enum' | 'cron'
  default?: string
  required: boolean
  options?: string[]
  placeholder?: string
}

export interface ProjectTemplate {
  id: string
  title: string
  summary: string
  params: ProjectTemplateParam[]
}

export interface ProjectTemplateFile {
  path: string
  content: string
}

export interface ProjectTemplateFieldError {
  param: string
  message: string
}

export class ProjectTemplateValidationError extends Error {
  readonly fields: ProjectTemplateFieldError[]
  constructor(message: string, fields: ProjectTemplateFieldError[]) {
    super(message)
    this.name = 'ProjectTemplateValidationError'
    this.fields = fields
  }
}

export const projectTemplateKeys = {
  all: ['project-templates'] as const,
}

export async function fetchProjectTemplates(
  signal?: AbortSignal,
): Promise<ProjectTemplate[]> {
  const res = await fetch('/api/v1/project-templates', { signal })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch templates failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { templates: ProjectTemplate[] }
  return body.templates
}

export function useProjectTemplates() {
  return useQuery({
    queryKey: projectTemplateKeys.all,
    queryFn: ({ signal }) => fetchProjectTemplates(signal),
    staleTime: 5 * 60_000,
  })
}

export async function renderProjectTemplate(
  id: string,
  params: Record<string, string>,
): Promise<ProjectTemplateFile[]> {
  const res = await fetch(
    `/api/v1/project-templates/${encodeURIComponent(id)}/render`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ params }),
    },
  )
  if (res.status === 422) {
    const body = (await res.json()) as {
      error: string
      fields: ProjectTemplateFieldError[]
    }
    throw new ProjectTemplateValidationError(body.error, body.fields)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `render template failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { files: ProjectTemplateFile[] }
  return body.files
}

export function useRenderProjectTemplate() {
  return useMutation({
    mutationFn: ({
      id,
      params,
    }: {
      id: string
      params: Record<string, string>
    }) => renderProjectTemplate(id, params),
  })
}
