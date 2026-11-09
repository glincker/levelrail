import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  cliCommand,
  cliSpec,
  curlCommand,
  shellQuote,
  tokenEnvVar,
  type CommandAction,
} from './cliCommands'

// Tests run from web/, so the repo root is one level up.
const repoFile = (rel: string) =>
  readFileSync(resolve(process.cwd(), '..', rel), 'utf8')

// Usage lines from the CLI reference's code blocks, binary name stripped, so a
// renamed verb or flag in the real CLI breaks this test.
function usageLines(): string[][] {
  const out: string[][] = []
  let inBlock = false
  for (const line of repoFile('docs/cli-reference.md').split('\n')) {
    if (line.startsWith('```')) {
      inBlock = !inBlock
      continue
    }
    if (inBlock && line.trim()) out.push(line.trim().split(/\s+/).slice(1))
  }
  return out
}

const tree = repoFile('internal/clitree/tree.go')

const every: CommandAction[] = [
  { kind: 'app.restart', app: 'web' },
  { kind: 'app.stop', app: 'web' },
  { kind: 'app.start', app: 'web' },
  { kind: 'app.rollback', app: 'web' },
  { kind: 'app.scale', app: 'web', replicas: 3 },
  { kind: 'deploy.rollback', app: 'web', deployId: 'dep_1' },
  { kind: 'database.start', database: 'pg' },
  { kind: 'database.stop', database: 'pg' },
  { kind: 'database.backup', database: 'pg', targetId: 'tgt_1' },
  { kind: 'domain.check', app: 'web', domain: 'a.example.com' },
  { kind: 'domain.remove', app: 'web', domain: 'a.example.com' },
  { kind: 'cert.renew', app: 'web', domain: 'a.example.com' },
  { kind: 'job.run', app: 'web', jobId: 'job_1' },
  { kind: 'node.cordon', nodeId: 'n1' },
  { kind: 'node.uncordon', nodeId: 'n1' },
]

describe('cliCommands against the real CLI', () => {
  const lines = usageLines()
  it.each(every.map((a) => [a.kind, a] as const))(
    '%s uses real verbs and flags',
    (_kind, action) => {
      const spec = cliSpec(action)
      for (const verb of spec.verbs) expect(tree).toContain(`"${verb}"`)
      const match = lines.find((tokens) =>
        spec.verbs.every((v, i) => (tokens[i] ?? '').split('|').includes(v)),
      )
      expect(match, spec.verbs.join(' ')).toBeDefined()
      for (const [flag] of spec.flags) {
        expect(match?.join(' ')).toContain(flag)
      }
    },
  )
})

describe('shell safety', () => {
  it('quotes hostile names so they stay one word', () => {
    expect(shellQuote('web')).toBe('web')
    expect(shellQuote('')).toBe("''")
    expect(shellQuote('my app')).toBe("'my app'")
    expect(shellQuote("it's")).toBe("'it'\\''s'")
    expect(shellQuote('x; rm -rf /')).toBe("'x; rm -rf /'")
    expect(shellQuote('$(whoami)')).toBe("'$(whoami)'")
    expect(shellQuote('`id`')).toBe("'`id`'")
  })

  it('builds commands with the brand binary and quoted args', () => {
    const cmd = cliCommand({ kind: 'app.restart', app: "a'b $(id)" }, 'acme')
    expect(cmd).toBe("acme-cli apps restart 'a'\\''b $(id)'")
    expect(
      cliCommand({ kind: 'app.scale', app: 'web', replicas: 2 }, 'acme'),
    ).toBe('acme-cli apps scale web --replicas 2')
  })

  it('uses a placeholder token and encodes path segments', () => {
    const curl = curlCommand(
      { kind: 'job.run', app: 'my app', jobId: "j'1" },
      'acme',
      'https://cp.example.com/',
    )
    expect(tokenEnvVar('acme')).toBe('ACME_TOKEN')
    expect(curl).toContain('"Authorization: Bearer $ACME_TOKEN"')
    expect(curl).toContain("/api/v1/apps/my%20app/jobs/j'\\''1/run")
    expect(curl).not.toMatch(/Bearer [A-Za-z0-9]{20,}/)
  })

  it('sends a JSON body quoted as one word', () => {
    const curl = curlCommand(
      { kind: 'domain.remove', app: 'web', domain: 'a.example.com' },
      'acme',
      'https://cp.example.com',
    )
    expect(curl).toContain(`-X PATCH`)
    expect(curl).toContain(`-d '{"remove":["a.example.com"]}'`)
  })

  it('has no API copy where one call would not match the CLI', () => {
    expect(
      curlCommand({ kind: 'app.rollback', app: 'web' }, 'acme', ''),
    ).toBeNull()
  })
})
