import { EnvApiError } from '../queries/envProtection'

export interface CliCommand {
  name: string
  subcommands: string[]
}

export type CliServerMessage =
  | { type: 'ready'; server: string }
  | { type: 'output'; stream: 'stdout' | 'stderr'; data: string }
  | { type: 'exit'; code: number }
  | { type: 'idle_timeout' }
  | { type: 'error'; message: string }

export type CliClientMessage =
  { type: 'run'; line: string } | { type: 'cancel' } | { type: 'ping' }

// A grant for this console is requested as a "manage" step-up on this
// pseudo app name, the existing scope shape the dialog and endpoints take.
export const CLI_CONSOLE_STEP_UP_APP = 'cli-console'

const COMMANDS_URL = '/api/v1/system/cli-console/commands'

export function cliConsoleSocketUrl(
  location: Pick<Location, 'protocol' | 'host'>,
): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${location.host}/api/v1/system/cli-console/session`
}

export function parseCliServerMessage(data: string): CliServerMessage | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(data)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null) return null
  const m = parsed as Record<string, unknown>
  switch (m.type) {
    case 'ready':
      return {
        type: 'ready',
        server: typeof m.server === 'string' ? m.server : '',
      }
    case 'output':
      if (typeof m.data !== 'string') return null
      return {
        type: 'output',
        stream: m.stream === 'stderr' ? 'stderr' : 'stdout',
        data: m.data,
      }
    case 'exit':
      return { type: 'exit', code: typeof m.code === 'number' ? m.code : 0 }
    case 'idle_timeout':
      return { type: 'idle_timeout' }
    case 'error':
      return {
        type: 'error',
        message: typeof m.message === 'string' ? m.message : '',
      }
    default:
      return null
  }
}

// Doubles as the step-up preflight: a WebSocket handshake refusal carries no
// readable body, this plain request does.
export async function fetchCliCommands(): Promise<CliCommand[]> {
  const res = await fetch(COMMANDS_URL)
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new EnvApiError(
      res.status,
      body?.error ?? `load commands failed: ${String(res.status)}`,
      body?.code ?? '',
    )
  }
  const body = (await res.json()) as { commands?: CliCommand[] }
  return (body.commands ?? []).map((c) => ({
    name: c.name,
    subcommands: c.subcommands,
  }))
}
