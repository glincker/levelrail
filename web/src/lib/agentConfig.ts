import type { Ability } from '../types/token'

export const AGENT_TOKEN_ENV = 'APP_API_TOKEN'
export const AGENT_URL_ENV = 'APP_API_URL'

export type AgentModeId = 'agent-core' | 'read-only' | 'standard' | 'full'

export interface AgentMode {
  id: AgentModeId
  label: string
  tools: string
  tokens: string
  hint: string
}

// Counts and token estimates come from docs/agent-tooling-audit.md
// (estimated size of the tools/list payload). They are estimates.
export const AGENT_MODES: readonly AgentMode[] = [
  {
    id: 'agent-core',
    label: 'Agent core',
    tools: 'about 15',
    tokens: 'about 2,500',
    hint: 'Ship and debug one app. Smallest context cost, the default.',
  },
  {
    id: 'read-only',
    label: 'Read only',
    tools: '109',
    tokens: '41,600',
    hint: 'Every read tool, nothing that changes state.',
  },
  {
    id: 'standard',
    label: 'Standard',
    tools: '135',
    tokens: '55,600',
    hint: 'Read and mutating tools, no destructive ones.',
  },
  {
    id: 'full',
    label: 'Full',
    tools: '144',
    tokens: '59,800',
    hint: 'Every tool, including destructive ones.',
  },
]

export interface AgentPreset {
  id: 'observer' | 'deployer' | 'operator'
  label: string
  abilities: Ability[]
  hint: string
}

// Keep in sync with agentPresets in cmd/levelrail-cli/tokens_create.go.
export const AGENT_PRESETS: readonly AgentPreset[] = [
  {
    id: 'observer',
    label: 'Read-only observer',
    abilities: ['read'],
    hint: 'Look at apps, logs, metrics and deploys. Cannot change anything.',
  },
  {
    id: 'deployer',
    label: 'Deployer',
    abilities: ['read', 'deploy'],
    hint: 'Also trigger deploys and rollbacks.',
  },
  {
    id: 'operator',
    label: 'Full operator',
    abilities: ['read', 'read:sensitive', 'write', 'write:sensitive', 'deploy'],
    hint: 'Also edit config, env vars and secrets. Never root.',
  },
]

export interface McpConfigInput {
  serverKey: string
  binary: string
  mode: AgentModeId
  apiURL: string
}

// Mirrors internal/agentinit.MCPJSON: the token is an env var reference,
// never a value.
export function buildMcpConfig(input: McpConfigInput): string {
  const args =
    input.mode === 'agent-core'
      ? ['--tool-profile', 'agent-core']
      : ['--mode', input.mode]
  const env: Record<string, string> = {
    [AGENT_TOKEN_ENV]: `\${${AGENT_TOKEN_ENV}}`,
  }
  if (input.apiURL) {
    env[AGENT_URL_ENV] = `\${${AGENT_URL_ENV}:-${input.apiURL}}`
  }
  const doc = {
    mcpServers: {
      [input.serverKey]: { command: input.binary, args, env },
    },
  }
  return JSON.stringify(doc, null, 2) + '\n'
}
