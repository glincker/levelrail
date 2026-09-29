import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { SSHNodeProvisionResource } from '../types/nodeSSHProvision'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const sshNodeProvisionKeys = {
  all: ['ssh-node-provisions'] as const,
  detail: (id: string) => [...sshNodeProvisionKeys.all, 'detail', id] as const,
}

// CreateSSHNodeProvisionInput mirrors internal/api's
// createSSHNodeProvisionRequest. auth is one of two credential shapes;
// neither is ever stored past the one provisioning call this starts.
export interface CreateSSHNodeProvisionInput {
  host: string
  port?: number
  username: string
  auth:
    | { type: 'key'; private_key: string; passphrase?: string }
    | { type: 'password'; password: string }
  name: string
  role: 'general' | 'build'
  control_plane_addr: string
}

async function createSSHNodeProvision(
  input: CreateSSHNodeProvisionInput,
): Promise<SSHNodeProvisionResource> {
  const res = await fetch('/api/v1/nodes/ssh-provision', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create ssh node provision failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as SSHNodeProvisionResource
}

export function useCreateSSHNodeProvision() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createSSHNodeProvision,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: sshNodeProvisionKeys.all,
      })
    },
  })
}

async function fetchSSHNodeProvision(
  id: string,
): Promise<SSHNodeProvisionResource> {
  const res = await fetch(
    `/api/v1/ssh-node-provisions/${encodeURIComponent(id)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch ssh node provision failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as SSHNodeProvisionResource
}

// SSH_PROVISION_POLL_INTERVAL_MS is shorter than node-provision's own
// PROVISION_POLL_INTERVAL_MS (nodeProvision.ts): there is no third-party
// provider API to be polite to here, and the operator is watching a live
// install log they'd like to feel responsive.
const SSH_PROVISION_POLL_INTERVAL_MS = 2_000

// Callers watch the returned status themselves (see AddNodeWizardSSH)
// and invalidate the node list once it turns "ready": refetchInterval
// must stay a pure function of query state, not a place to run side
// effects, the same rule useNodeProvision's own doc comment gives.
export function useSSHNodeProvision(id: string | null) {
  return useQuery({
    queryKey: sshNodeProvisionKeys.detail(id ?? ''),
    queryFn: () => fetchSSHNodeProvision(id ?? ''),
    enabled: id !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'ready' || status === 'failed'
        ? false
        : SSH_PROVISION_POLL_INTERVAL_MS
    },
  })
}
