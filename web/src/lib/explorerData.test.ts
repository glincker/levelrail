import { describe, expect, it } from 'vitest'
import {
  columnKind,
  parseFilterInput,
  prettyJson,
  selectStatement,
  toCsv,
  toInsert,
  toJson,
} from './explorerData'
import { buildTreeItems, formatRowEstimate } from './explorerTree'
import type { DbSchemaNode } from '../types/databaseViewer'

const COLUMNS = ['id', 'name', 'note', 'active', 'meta']
const KINDS = ['number', 'text', 'text', 'boolean', 'json'] as const
const ROWS = [
  ['1', 'Ada, "the" first', null, 't', '{"a":1}'],
  ['2', "O'Neil", '', 'f', 'not json'],
  ['3', 'line\nbreak', 'x', null, null],
]

describe('toCsv', () => {
  it('quotes commas, quotes and newlines and keeps NULL apart from empty', () => {
    const csv = toCsv(COLUMNS, ROWS)
    const lines = csv.split('\n')
    expect(lines[0]).toBe('id,name,note,active,meta')
    expect(lines[1]).toBe('1,"Ada, ""the"" first",,t,"{""a"":1}"')
    expect(lines[2]).toBe(`2,O'Neil,"",f,not json`)
    expect(csv).toContain('"line\nbreak"')
  })
})

describe('toJson', () => {
  it('types numbers, booleans and json and keeps null', () => {
    const parsed = JSON.parse(toJson(COLUMNS, ROWS, [...KINDS])) as Record<
      string,
      unknown
    >[]
    expect(parsed[0]).toEqual({
      id: 1,
      name: 'Ada, "the" first',
      note: null,
      active: true,
      meta: { a: 1 },
    })
    expect(parsed[1]?.note).toBe('')
    expect(parsed[1]?.meta).toBe('not json')
    expect(parsed[2]?.active).toBeNull()
  })
})

describe('toInsert', () => {
  it('builds postgres statements with doubled quotes and bare numbers', () => {
    const sql = toInsert('postgres', 'public', 'people', COLUMNS, ROWS, [
      ...KINDS,
    ])
    const [first, second] = sql.split('\n')
    expect(first).toBe(
      `INSERT INTO "public"."people" ("id", "name", "note", "active", "meta") VALUES (1, 'Ada, "the" first', NULL, TRUE, '{"a":1}');`,
    )
    expect(second).toContain(`'O''Neil', '', FALSE`)
  })

  it('quotes mysql identifiers and escapes backslashes', () => {
    const sql = toInsert('mysql', 'app', 'files', ['path'], [['C:\\tmp']])
    expect(sql).toBe("INSERT INTO `app`.`files` (`path`) VALUES ('C:\\\\tmp');")
  })

  it('never emits a number literal for non numeric text in a number column', () => {
    const sql = toInsert(
      'postgres',
      's',
      't',
      ['n'],
      [['1; DROP TABLE x']],
      ['number'],
    )
    expect(sql).toContain(`'1; DROP TABLE x'`)
  })
})

describe('parseFilterInput', () => {
  it('maps the filter box syntax to operators', () => {
    expect(parseFilterInput('a', '  ')).toBeNull()
    expect(parseFilterInput('a', 'foo')).toEqual({
      column: 'a',
      op: 'contains',
      value: 'foo',
    })
    expect(parseFilterInput('a', '=foo')).toEqual({
      column: 'a',
      op: 'equals',
      value: 'foo',
    })
    expect(parseFilterInput('a', 'IS:NULL')?.op).toBe('is_null')
    expect(parseFilterInput('a', 'not:null')?.op).toBe('not_null')
  })
})

describe('helpers', () => {
  it('classifies column types', () => {
    expect(columnKind('integer')).toBe('number')
    expect(columnKind('numeric(10,2)')).toBe('number')
    expect(columnKind('jsonb')).toBe('json')
    expect(columnKind('timestamp with time zone')).toBe('datetime')
    expect(columnKind('character varying(255)')).toBe('text')
    expect(columnKind('uuid')).toBe('uuid')
    expect(columnKind('boolean')).toBe('boolean')
    expect(columnKind('bytea')).toBe('binary')
  })

  it('pretty prints objects and arrays only', () => {
    expect(prettyJson('{"a":[1,2]}')).toBe('{\n  "a": [\n    1,\n    2\n  ]\n}')
    expect(prettyJson('42')).toBeUndefined()
    expect(prettyJson('plain text')).toBeUndefined()
  })

  it('builds the console select with a limit', () => {
    expect(selectStatement('postgres', 'public', 'users', 100)).toBe(
      'SELECT * FROM "public"."users" LIMIT 100;',
    )
  })

  it('abbreviates row estimates', () => {
    expect(formatRowEstimate(950)).toBe('950')
    expect(formatRowEstimate(1500)).toBe('1.5k')
    expect(formatRowEstimate(2_400_000)).toBe('2.4M')
  })
})

describe('buildTreeItems', () => {
  const schemas: DbSchemaNode[] = [
    {
      name: 'public',
      tables: [
        { name: 'users', kind: 'table' },
        { name: 'user_roles', kind: 'table' },
        { name: 'orders', kind: 'table' },
      ].map((t) => ({
        ...t,
        row_estimate: 0,
        size_bytes: 0,
        columns: [],
        indexes: [],
      })),
    },
    {
      name: 'audit',
      tables: [
        {
          name: 'events',
          kind: 'table',
          row_estimate: 0,
          size_bytes: 0,
          columns: [],
          indexes: [],
        },
      ],
    },
  ]

  it('groups by schema and honours collapse', () => {
    const open = buildTreeItems(schemas, '', new Set())
    expect(open.map((i) => i.type)).toEqual([
      'schema',
      'table',
      'table',
      'table',
      'schema',
      'table',
    ])
    const collapsed = buildTreeItems(schemas, '', new Set(['public']))
    expect(collapsed.filter((i) => i.type === 'table')).toHaveLength(1)
  })

  it('filters by every token across schema and name and ignores collapse', () => {
    const hits = buildTreeItems(schemas, 'user rol', new Set(['public']))
    const names = hits.flatMap((i) =>
      i.type === 'table' ? [i.table.name] : [],
    )
    expect(names).toEqual(['user_roles'])
    const bySchema = buildTreeItems(schemas, 'audit', new Set())
    expect(bySchema).toHaveLength(2)
  })
})
