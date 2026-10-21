import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface CertExpiryEmail {
  enabled: boolean
  available: boolean
  reason?:
    'no_email_provider' | 'address_not_verified' | 'no_address' | 'no_session'
}

const key = ['alert-defaults', 'cert-expiry', 'email'] as const
const url = '/api/v1/alert-defaults/cert-expiry/email'

async function failure(res: Response): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `cert expiry email failed: ${res.status}`),
  )
}

export function useCertExpiryEmail() {
  return useQuery({
    queryKey: key,
    queryFn: async (): Promise<CertExpiryEmail> => {
      const res = await fetch(url)
      if (!res.ok) throw await failure(res)
      return (await res.json()) as CertExpiryEmail
    },
  })
}

export function useSetCertExpiryEmail() {
  const qc = useQueryClient()
  return useMutation<CertExpiryEmail, ApiError, boolean>({
    mutationFn: async (enabled) => {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      })
      if (!res.ok) throw await failure(res)
      return (await res.json()) as CertExpiryEmail
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: key })
    },
  })
}
