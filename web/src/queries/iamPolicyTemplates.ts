import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { iamPolicyKeys } from './iamPolicies'
import type {
  PolicyDocument,
  PolicyResource,
  PrincipalType,
} from './iamPolicies'

export interface PolicyTemplateParam {
  name: string
  description: string
  required: boolean
}

export interface PolicyTemplate {
  id: string
  name: string
  description: string
  params: PolicyTemplateParam[]
  document: PolicyDocument
}

export interface PolicyTemplateList {
  version: number
  templates: PolicyTemplate[]
}

export interface ApplyTemplateRequest {
  id: string
  params: Record<string, string>
  attach?: { principal_type: PrincipalType; principal_id: string }
}

export interface ApplyTemplateResponse {
  policy: PolicyResource
  attached: boolean
}

export const policyTemplateKeys = {
  all: ['iam-policy-templates'] as const,
}

export async function fetchPolicyTemplates(): Promise<PolicyTemplateList> {
  const res = await fetch('/api/v1/iam/policy-templates')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch policy templates failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as PolicyTemplateList
}

export function policyTemplatesQueryOptions() {
  return queryOptions({
    queryKey: policyTemplateKeys.all,
    queryFn: fetchPolicyTemplates,
  })
}

export async function applyPolicyTemplate(
  req: ApplyTemplateRequest,
): Promise<ApplyTemplateResponse> {
  const res = await fetch(
    `/api/v1/iam/policy-templates/${encodeURIComponent(req.id)}/apply`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ params: req.params, attach: req.attach }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `apply policy template failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ApplyTemplateResponse
}

export function useApplyPolicyTemplate() {
  const queryClient = useQueryClient()
  return useMutation<ApplyTemplateResponse, ApiError, ApplyTemplateRequest>({
    mutationFn: applyPolicyTemplate,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: iamPolicyKeys.all })
    },
  })
}

// Fills {param} placeholders the server leaves in a parameterized template.
export function renderTemplateDocument(
  template: PolicyTemplate,
  params: Record<string, string>,
): PolicyDocument {
  const fill = (value: string) =>
    value.replace(/\{(\w+)\}/g, (match, key: string) => {
      const filled = params[key]
      return filled ? filled : match
    })
  return {
    Statement: template.document.Statement.map((s) => ({
      Effect: s.Effect,
      Action: s.Action,
      Resource: s.Resource.map(fill),
    })),
  }
}
