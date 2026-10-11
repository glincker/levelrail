import type { CliCommand } from './cliConsoleProtocol'

export type QuickActionLevel = 'read' | 'change' | 'destructive'
export type QuickActionGroup = 'overview' | 'apps' | 'platform'
export type QuickParamKind = 'app' | 'node' | 'text'
export type QuickParamName = 'app' | 'node' | 'image'
export type QuickActionId =
  | 'status'
  | 'version'
  | 'appsList'
  | 'deploysList'
  | 'nodesList'
  | 'databasesList'
  | 'appStatus'
  | 'appLogs'
  | 'appDeploys'
  | 'nodeGet'
  | 'appRestart'
  | 'appRedeploy'
  | 'appRollback'
  | 'systemRestart'

export interface QuickParam {
  name: QuickParamName
  kind: QuickParamKind
}

export interface QuickAction {
  id: QuickActionId
  group: QuickActionGroup
  level: QuickActionLevel
  // Literal words plus `{param}` placeholders, one per argv word.
  template: readonly string[]
  params: readonly QuickParam[]
}

const APP: QuickParam = { name: 'app', kind: 'app' }
const NODE: QuickParam = { name: 'node', kind: 'node' }
const IMAGE: QuickParam = { name: 'image', kind: 'text' }

export const QUICK_ACTION_GROUPS: readonly QuickActionGroup[] = [
  'overview',
  'apps',
  'platform',
]

export const QUICK_ACTIONS: readonly QuickAction[] = [
  {
    id: 'status',
    group: 'overview',
    level: 'read',
    template: ['status'],
    params: [],
  },
  {
    id: 'version',
    group: 'overview',
    level: 'read',
    template: ['version'],
    params: [],
  },
  {
    id: 'appsList',
    group: 'overview',
    level: 'read',
    template: ['apps', 'list'],
    params: [],
  },
  {
    id: 'deploysList',
    group: 'overview',
    level: 'read',
    template: ['deployments', 'list'],
    params: [],
  },
  {
    id: 'nodesList',
    group: 'overview',
    level: 'read',
    template: ['nodes', 'list'],
    params: [],
  },
  {
    id: 'databasesList',
    group: 'overview',
    level: 'read',
    template: ['databases', 'list'],
    params: [],
  },
  {
    id: 'appStatus',
    group: 'apps',
    level: 'read',
    template: ['apps', 'status', '{app}'],
    params: [APP],
  },
  {
    id: 'appLogs',
    group: 'apps',
    level: 'read',
    template: ['apps', 'logs', '{app}'],
    params: [APP],
  },
  {
    id: 'appDeploys',
    group: 'apps',
    level: 'read',
    template: ['apps', 'deploys', 'list', '{app}'],
    params: [APP],
  },
  {
    id: 'nodeGet',
    group: 'platform',
    level: 'read',
    template: ['nodes', 'get', '{node}'],
    params: [NODE],
  },
  {
    id: 'appRestart',
    group: 'apps',
    level: 'change',
    template: ['apps', 'restart', '{app}'],
    params: [APP],
  },
  {
    id: 'appRedeploy',
    group: 'apps',
    level: 'change',
    template: ['apps', 'deploy', '{app}', '--pull'],
    params: [APP],
  },
  {
    id: 'appRollback',
    group: 'apps',
    level: 'destructive',
    template: ['apps', 'rollback', '{app}', '--image', '{image}'],
    params: [APP, IMAGE],
  },
  {
    id: 'systemRestart',
    group: 'platform',
    level: 'destructive',
    template: ['system', 'restart', '--yes'],
    params: [],
  },
]

const BARE_WORD = /^[A-Za-z0-9_@%+=:,./][A-Za-z0-9_@%+=:,./-]*$/
const UNSAFE_SEQUENCE = /`|\$\(|\$\{/

// Mirrors the server's ParseLine refusals, so a button never builds a line it rejects.
function unsendable(value: string): boolean {
  if (UNSAFE_SEQUENCE.test(value)) return true
  for (const ch of value) {
    const code = ch.charCodeAt(0)
    if ((code < 0x20 && code !== 0x09) || code === 0x7f) return true
  }
  return false
}

/** Returns the word quoted for the console's POSIX-style parser, or null if it cannot be sent. */
export function quoteArg(value: string): string | null {
  if (value === '' || value.startsWith('-') || unsendable(value)) return null
  if (BARE_WORD.test(value)) return value
  return `'${value.replace(/'/g, `'\\''`)}'`
}

/** Builds the exact line to type, or null while a param is empty or unsendable. */
export function buildLine(
  action: QuickAction,
  values: Readonly<Record<string, string>>,
): string | null {
  const words: string[] = []
  for (const word of action.template) {
    const match = /^\{(\w+)\}$/.exec(word)
    if (!match) {
      words.push(word)
      continue
    }
    const quoted = quoteArg((values[match[1] ?? ''] ?? '').trim())
    if (quoted === null) return null
    words.push(quoted)
  }
  return words.join(' ')
}

function commandPrefix(action: QuickAction): string[] {
  const words: string[] = []
  for (const word of action.template) {
    if (word.startsWith('-') || word.startsWith('{')) break
    words.push(word)
  }
  return words
}

/** Keeps only actions whose command and first subcommand the server lists. */
export function filterQuickActions(
  actions: readonly QuickAction[],
  commands: readonly CliCommand[],
): QuickAction[] {
  const byName = new Map(commands.map((c) => [c.name, c.subcommands]))
  return actions.filter((action) => {
    const [top, sub] = commandPrefix(action)
    if (top === undefined) return false
    const subs = byName.get(top)
    if (subs === undefined) return false
    if (sub === undefined || subs.length === 0) return true
    return subs.includes(sub)
  })
}

export interface NameRange {
  name: string
  start: number
  end: number
}

const NAME_CHAR = /[A-Za-z0-9_.-]/

/** Finds exact whole-word occurrences of known names in one output line. */
export function findNameRanges(
  line: string,
  names: readonly string[],
): NameRange[] {
  const found: NameRange[] = []
  for (const name of new Set(names)) {
    if (name === '') continue
    let from = 0
    for (;;) {
      const at = line.indexOf(name, from)
      if (at === -1) break
      const end = at + name.length
      const before = at === 0 ? '' : (line[at - 1] ?? '')
      const after = end >= line.length ? '' : (line[end] ?? '')
      if (!NAME_CHAR.test(before) && !NAME_CHAR.test(after)) {
        found.push({ name, start: at, end })
      }
      from = at + 1
    }
  }
  return found.sort((a, b) => a.start - b.start)
}
