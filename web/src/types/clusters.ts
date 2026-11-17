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
  allow_cluster_admin?: boolean
}

export interface DeployK8sInput {
  app: string
  image: string
  port: number
  replicas?: number
  namespace?: string
  host?: string
  env?: Record<string, string>
  allow_root?: boolean
  pull_secret?: string
}

export interface PullSecret {
  name: string
  registry: string
}

export interface AddPullSecretInput {
  name: string
  registry: string
  username: string
  password: string
}

export interface K8sDeployment {
  app: string
  namespace: string
  image: string
  port: number
  replicas: number
  host?: string
  allow_root: boolean
  pull_secret?: string
  objects: string[]
}

export interface K8sCondition {
  type: string
  status: string
  reason?: string
  message?: string
}

export interface K8sPod {
  name: string
  phase: string
  reason?: string
  message?: string
}

export interface K8sDeploymentStatus {
  app: string
  namespace: string
  found: boolean
  ready: boolean
  replicas: number
  ready_replicas: number
  updated_replicas: number
  conditions: K8sCondition[]
  pods: K8sPod[]
  ingress_address?: string
  reason?: string
}
