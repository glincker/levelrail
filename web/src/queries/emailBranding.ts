// GET/PUT /api/v1/settings/email/branding and its preview endpoint
// (internal/api/email_branding.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const emailBrandingKey = ['email-settings', 'branding'] as const

export interface EmailFooterLink {
  label: string
  url: string
}

export interface EmailBranding {
  logo_url: string
  brand_color: string
  footer_text: string
  footer_address: string
  footer_links: EmailFooterLink[]
  show_powered_by: boolean
  default_brand_color?: string
  max_footer_links?: number
  variables?: string[]
}

export interface EmailBrandingPreview {
  subject: string
  html: string
  text: string
}

async function send<T>(
  method: 'GET' | 'PUT' | 'POST',
  path: string,
  body?: EmailBranding,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `email branding request failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as T
}

export function emailBrandingQueryOptions() {
  return queryOptions({
    queryKey: emailBrandingKey,
    queryFn: () =>
      send<EmailBranding>('GET', '/api/v1/settings/email/branding'),
    staleTime: 60_000,
  })
}

export function useEmailBranding() {
  return useQuery(emailBrandingQueryOptions())
}

export function useUpdateEmailBranding() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (b: EmailBranding) =>
      send<EmailBranding>('PUT', '/api/v1/settings/email/branding', b),
    onSuccess: (saved) => {
      queryClient.setQueryData(emailBrandingKey, saved)
    },
  })
}

export function usePreviewEmailBranding() {
  return useMutation({
    mutationFn: (b: EmailBranding) =>
      send<EmailBrandingPreview>(
        'POST',
        '/api/v1/settings/email/branding/preview',
        b,
      ),
  })
}
