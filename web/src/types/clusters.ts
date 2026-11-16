// Shapes of the /api/v1/clusters endpoints (internal/api/k8s_clusters*.go). No secret fields exist.
export type ClusterAuthMode = 'token' | 'client_cert'

export interface Cluster {
  name: string
  server: string
  auth_mode: ClusterAuthMode
  ca_fingerprint?: string
  context?: string
  created_by?: string
  created_at?: string
  last_checked_at?: string
  last_check_class?: string
}

export interface ClusterList {
  clusters: Cluster[]
}

export interface ClusterTestResult {
  ok: boolean
  class: string
  cluster_admin?: boolean
}

export interface ClusterNode {
  name: string
  ready: boolean
  roles: string[]
  kubelet_version: string
}

export interface ClusterNamespace {
  name: string
  phase: string
}

export interface ConnectClusterInput {
  name: string
  kubeconfig?: string
  context?: string
  server?: string
  token?: string
  ca?: string
}
