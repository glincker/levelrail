import type { NodeJoinTokenResponse } from '../types/nodeDetail'

// The CLI flags match cmd/levelrail-cli/builder.go.
export function buildBuilderJoinCommand(
  server: string,
  res: NodeJoinTokenResponse,
): string {
  const parts = [
    'levelrail builder join',
    `--server ${server}`,
    `--token ${res.token}`,
  ]
  if (res.control_plane_addr)
    parts.push(`--agent-addr ${res.control_plane_addr}`)
  if (res.ca_fingerprint) parts.push(`--ca-fingerprint ${res.ca_fingerprint}`)
  return parts.join(' ')
}
