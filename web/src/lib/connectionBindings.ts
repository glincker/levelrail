// Binding roles and the connection capabilities that can fill each one (mirrors internal/connhub/resolve.go).

import type { Connection } from '../types/connections'

export const BINDING_ROLES = [
  'source',
  'deploy_report',
  'registry_pull',
  'notify',
  'secrets',
  'dns',
] as const

export type BindingRole = (typeof BINDING_ROLES)[number]

const ROLE_CAPABILITIES: Record<BindingRole, readonly string[]> = {
  source: ['source'],
  deploy_report: ['status', 'deployments'],
  registry_pull: ['registry_pull'],
  notify: ['notify'],
  secrets: ['secrets'],
  dns: ['dns'],
}

export function isBindingRole(role: string): role is BindingRole {
  return (BINDING_ROLES as readonly string[]).includes(role)
}

export function canFillRole(conn: Connection, role: BindingRole): boolean {
  const caps = conn.capabilities ?? []
  return ROLE_CAPABILITIES[role].some((c) => caps.includes(c))
}

export function rolesForConnection(conn: Connection): BindingRole[] {
  return BINDING_ROLES.filter((r) => canFillRole(conn, r))
}

export function connectionsForRole(
  conns: readonly Connection[],
  role: BindingRole,
): Connection[] {
  return conns.filter((c) => canFillRole(c, role))
}
