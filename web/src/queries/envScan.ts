import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface EnvScanFinding {
  var: string
  rule_id: string
  description: string
  fingerprint: string
  allowed?: boolean
}

export async function scanEnvValues(
  appName: string,
  vars: Record<string, string>,
): Promise<EnvScanFinding[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/env/scan`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ vars }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `scan env failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { findings: EnvScanFinding[] }
  return body.findings.filter((f) => !f.allowed)
}

export function useScanEnvValues(appName: string) {
  return useMutation({
    mutationFn: (vars: Record<string, string>) => scanEnvValues(appName, vars),
  })
}
