import { useQuery } from '@tanstack/react-query'
import { readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

export type NodeResourceKind = 'app' | 'database'

export interface NodeResource {
  kind: NodeResourceKind
  name: string
  detail: string
  status: 'active' | 'suspended'
  replicas?: number
  memory_bytes?: number
  nano_cpus?: number
}

export interface NodeResources {
  node_id: string
  resources: NodeResource[]
}

export async function fetchNodeResources(
  nodeId: string,
): Promise<NodeResources> {
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(nodeId)}/resources`,
  )
  if (!res.ok) {
    throw new Error(
      await readErrorMessage(res, `node resources failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeResources
}

/** useNodeResources lists the apps and databases placed on a node that the caller may read. */
export function useNodeResources(nodeId: string) {
  return useQuery({
    queryKey: [...nodeKeys.detail(nodeId), 'resources'] as const,
    queryFn: () => fetchNodeResources(nodeId),
    retry: false,
  })
}
