import { describe, expect, it } from 'vitest'
import {
  QUICK_ACTIONS,
  buildLine,
  filterQuickActions,
  findNameRanges,
  quoteArg,
  type QuickAction,
} from './cliConsoleQuickActions'

function action(id: string): QuickAction {
  const found = QUICK_ACTIONS.find((a) => a.id === id)
  if (!found) throw new Error(`no action ${id}`)
  return found
}

describe('quoteArg', () => {
  const cases: Array<[string, string, string | null]> = [
    ['plain name', 'web-api', 'web-api'],
    ['image reference', 'ghcr.io/org/app:v1.2', 'ghcr.io/org/app:v1.2'],
    ['space', 'my app', `'my app'`],
    ['single quote', `it's`, `'it'\\''s'`],
    ['double quote', 'a"b', `'a"b'`],
    ['empty', '', null],
    ['leading dash looks like a flag', '--token', null],
    ['backtick', 'a`b', null],
    ['command substitution', '$(id)', null],
    ['brace expansion', '${HOME}', null],
    ['newline', 'a\nb', null],
  ]
  it.each(cases)('%s', (_label, input, want) => {
    expect(quoteArg(input)).toBe(want)
  })
})

describe('buildLine', () => {
  const cases: Array<[string, string, Record<string, string>, string | null]> =
    [
      ['no params', 'status', {}, 'status'],
      ['app restart', 'appRestart', { app: 'web' }, 'apps restart web'],
      [
        'redeploy keeps --pull',
        'appRedeploy',
        { app: 'web' },
        'apps deploy web --pull',
      ],
      ['name with space', 'appLogs', { app: 'my app' }, `apps logs 'my app'`],
      [
        'name with quote',
        'appLogs',
        { app: `o'neil` },
        `apps logs 'o'\\''neil'`,
      ],
      [
        'two params',
        'appRollback',
        { app: 'web', image: 'web:1' },
        'apps rollback web --image web:1',
      ],
      ['missing param', 'appRollback', { app: 'web' }, null],
      ['blank param', 'appRestart', { app: '  ' }, null],
      ['unsendable param', 'appRestart', { app: 'a`b' }, null],
      ['node', 'nodeGet', { node: 'node-1' }, 'nodes get node-1'],
      ['system restart', 'systemRestart', {}, 'system restart --yes'],
    ]
  it.each(cases)('%s', (_label, id, values, want) => {
    expect(buildLine(action(id), values)).toBe(want)
  })
})

describe('filterQuickActions', () => {
  const commands = [
    { name: 'status', subcommands: [] },
    { name: 'apps', subcommands: ['list', 'restart', 'status'] },
    { name: 'system', subcommands: ['restart'] },
  ]
  it('keeps only actions the server lists', () => {
    const ids = filterQuickActions(QUICK_ACTIONS, commands).map((a) => a.id)
    expect(ids).toEqual([
      'status',
      'appsList',
      'appStatus',
      'appRestart',
      'systemRestart',
    ])
  })
  it('returns nothing before the allowlist is known', () => {
    expect(filterQuickActions(QUICK_ACTIONS, [])).toEqual([])
  })
  it('drops a command whose subcommand was denied', () => {
    const denied = [{ name: 'system', subcommands: ['address'] }]
    expect(filterQuickActions(QUICK_ACTIONS, denied)).toEqual([])
  })
})

describe('findNameRanges', () => {
  const cases: Array<[string, string, string[], Array<[string, number]>]> = [
    ['whole word', 'web  running', ['web'], [['web', 0]]],
    ['prefix of another name is skipped', 'web-api up', ['web'], []],
    [
      'both names',
      'web web-api',
      ['web', 'web-api'],
      [
        ['web', 0],
        ['web-api', 4],
      ],
    ],
    ['substring inside word', 'cobweb', ['web'], []],
    ['next to punctuation', '(web),', ['web'], [['web', 1]]],
    ['empty name ignored', 'a b', [''], []],
  ]
  it.each(cases)('%s', (_label, line, names, want) => {
    const got = findNameRanges(line, names).map((r) => [r.name, r.start])
    expect(got).toEqual(want)
  })
})
