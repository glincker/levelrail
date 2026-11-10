// Single source of truth for "copy as command": each dashboard action maps to
// the CLI verb path, positional args and flags it really takes, plus the REST
// call behind it. Params never carry secret values, so nothing secret can leak.

export type CommandAction =
  | {
      kind: 'app.restart' | 'app.stop' | 'app.start' | 'app.rollback'
      app: string
    }
  | { kind: 'app.scale'; app: string; replicas: number }
  | { kind: 'deploy.rollback'; app: string; deployId: string }
  | { kind: 'database.start' | 'database.stop'; database: string }
  | { kind: 'database.backup'; database: string; targetId?: string }
  | {
      kind: 'domain.check' | 'domain.remove' | 'cert.renew'
      app: string
      domain: string
    }
  | { kind: 'job.run'; app: string; jobId: string }
  | { kind: 'node.cordon' | 'node.uncordon'; nodeId: string }

export type CommandKind = CommandAction['kind']

export interface CliSpec {
  verbs: string[]
  args: string[]
  flags: [string, string | null][]
}

export interface ApiSpec {
  method: 'GET' | 'POST' | 'PATCH'
  path: string
  body?: Record<string, unknown>
}

const SAFE_ARG = /^[A-Za-z0-9._/:=@%+,-]+$/

/** shellQuote returns s as one POSIX shell word, single quoting anything unsafe. */
export function shellQuote(s: string): string {
  if (s !== '' && SAFE_ARG.test(s)) return s
  return `'${s.replace(/'/g, `'\\''`)}'`
}

const seg = encodeURIComponent

/** cliSpec maps an action to its CLI verbs, positional args and flags. */
export function cliSpec(a: CommandAction): CliSpec {
  switch (a.kind) {
    case 'app.restart':
      return { verbs: ['apps', 'restart'], args: [a.app], flags: [] }
    case 'app.stop':
      return { verbs: ['apps', 'stop'], args: [a.app], flags: [] }
    case 'app.start':
      return { verbs: ['apps', 'start'], args: [a.app], flags: [] }
    case 'app.rollback':
      return { verbs: ['apps', 'rollback'], args: [a.app], flags: [] }
    case 'app.scale':
      return {
        verbs: ['apps', 'scale'],
        args: [a.app],
        flags: [['--replicas', String(a.replicas)]],
      }
    case 'deploy.rollback':
      return {
        verbs: ['apps', 'deploys', 'rollback-to'],
        args: [a.app, a.deployId],
        flags: [],
      }
    case 'database.start':
      return { verbs: ['databases', 'start'], args: [a.database], flags: [] }
    case 'database.stop':
      return { verbs: ['databases', 'stop'], args: [a.database], flags: [] }
    case 'database.backup':
      return {
        verbs: ['backups', 'trigger'],
        args: [a.database],
        flags: a.targetId ? [['--target', a.targetId]] : [],
      }
    case 'domain.check':
      return { verbs: ['domains', 'check'], args: [a.app, a.domain], flags: [] }
    case 'domain.remove':
      return {
        verbs: ['apps', 'domains', 'remove'],
        args: [a.app, a.domain],
        flags: [],
      }
    case 'cert.renew':
      return {
        verbs: ['domains', 'tls-cert', 'renew'],
        args: [a.app, a.domain],
        flags: [],
      }
    case 'job.run':
      return { verbs: ['jobs', 'run'], args: [a.app, a.jobId], flags: [] }
    case 'node.cordon':
      return { verbs: ['nodes', 'cordon'], args: [a.nodeId], flags: [] }
    case 'node.uncordon':
      return { verbs: ['nodes', 'uncordon'], args: [a.nodeId], flags: [] }
  }
}

/** apiSpec maps an action to its REST call, or null when it has no single call. */
export function apiSpec(a: CommandAction): ApiSpec | null {
  switch (a.kind) {
    case 'app.restart':
    case 'app.stop':
    case 'app.start':
      return {
        method: 'POST',
        path: `/api/v1/apps/${seg(a.app)}/${a.kind.slice(4)}`,
      }
    case 'deploy.rollback':
      return {
        method: 'POST',
        path: `/api/v1/apps/${seg(a.app)}/deploys/${seg(a.deployId)}/rollback`,
      }
    case 'database.start':
    case 'database.stop':
      return {
        method: 'POST',
        path: `/api/v1/databases/${seg(a.database)}/${a.kind.slice(9)}`,
      }
    case 'database.backup':
      return {
        method: 'POST',
        path: `/api/v1/databases/${seg(a.database)}/backups`,
        body: a.targetId ? { target_id: a.targetId } : {},
      }
    case 'domain.check':
      return {
        method: 'GET',
        path: `/api/v1/apps/${seg(a.app)}/domains/${seg(a.domain)}/check?refresh=true`,
      }
    case 'domain.remove':
      return {
        method: 'PATCH',
        path: `/api/v1/apps/${seg(a.app)}/domains`,
        body: { remove: [a.domain] },
      }
    case 'cert.renew':
      return {
        method: 'POST',
        path: `/api/v1/apps/${seg(a.app)}/domains/${seg(a.domain)}/cert/renew`,
      }
    case 'job.run':
      return {
        method: 'POST',
        path: `/api/v1/apps/${seg(a.app)}/jobs/${seg(a.jobId)}/run`,
      }
    case 'node.cordon':
    case 'node.uncordon':
      return {
        method: 'POST',
        path: `/api/v1/nodes/${seg(a.nodeId)}/${a.kind.slice(5)}`,
      }
    // Scale and rollback rewrite the whole app on the API side, so the CLI is the honest copy.
    case 'app.scale':
    case 'app.rollback':
      return null
  }
}

/** cliBinary is the CLI's binary name, derived from the brand's binary name. */
export function cliBinary(brandBinary: string): string {
  return `${brandBinary}-cli`
}

/** tokenEnvVar is the placeholder token variable the CLI itself reads. */
export function tokenEnvVar(brandBinary: string): string {
  return `${brandBinary.toUpperCase().replace(/[^A-Z0-9]/g, '_')}_TOKEN`
}

/** cliCommand renders the full, shell-safe CLI command for an action. */
export function cliCommand(a: CommandAction, brandBinary: string): string {
  const spec = cliSpec(a)
  const parts = [
    cliBinary(brandBinary),
    ...spec.verbs,
    ...spec.args.map(shellQuote),
  ]
  for (const [flag, value] of spec.flags) {
    parts.push(flag)
    if (value !== null) parts.push(shellQuote(value))
  }
  return parts.join(' ')
}

/** curlCommand renders the REST call with a placeholder token, never a real one. */
export function curlCommand(
  a: CommandAction,
  brandBinary: string,
  origin: string,
): string | null {
  const spec = apiSpec(a)
  if (!spec) return null
  const parts = [
    'curl',
    '-X',
    spec.method,
    '-H',
    `"Authorization: Bearer $${tokenEnvVar(brandBinary)}"`,
  ]
  if (spec.body) {
    parts.push('-H', shellQuote('Content-Type: application/json'))
    parts.push('-d', shellQuote(JSON.stringify(spec.body)))
  }
  parts.push(shellQuote(`${origin.replace(/\/+$/, '')}${spec.path}`))
  return parts.join(' ')
}
