import { describe, expect, it } from 'vitest'
import type { DbSchemaNode, DbTable } from '../../types/databaseViewer'
import { previousWord, suggest, tokenAt } from './complete'

const table = (name: string, cols: [string, string][]): DbTable => ({
  name,
  kind: 'table',
  row_estimate: 0,
  size_bytes: 0,
  indexes: [],
  columns: cols.map(([n, type]) => ({
    name: n,
    type,
    nullable: false,
    primary_key: n === 'id',
  })),
})

const schemas: DbSchemaNode[] = [
  {
    name: 'public',
    tables: [
      table('users', [
        ['id', 'uuid'],
        ['email', 'text'],
      ]),
      table('orders', [
        ['id', 'uuid'],
        ['user_id', 'uuid'],
        ['Total Price', 'numeric'],
      ]),
    ],
  },
  { name: 'audit', tables: [table('events', [['id', 'bigint']])] },
]

const labels = (r: ReturnType<typeof suggest>) => r.map((s) => s.label)

describe('suggest', () => {
  it('offers only tables after FROM and JOIN, schema qualified too', () => {
    const r = suggest(
      'us',
      schemas,
      false,
      previousWord('select * from us', 14),
    )
    expect(labels(r)).toContain('users')
    expect(r.every((s) => s.kind === 'table')).toBe(true)
    const j = suggest('ev', schemas, false, 'join')
    expect(j.map((s) => s.insert)).toContain('audit.events')
  })

  it('offers columns with types and keywords in SELECT and WHERE', () => {
    const r = suggest('em', schemas, false, 'select')
    expect(r[0]).toMatchObject({ label: 'email', kind: 'column' })
    expect(r[0]?.detail).toContain('text')
    expect(labels(suggest('wh', schemas, false, 'select'))).toContain('WHERE')
    expect(labels(suggest('user_', schemas, false, 'where'))).toEqual([
      'user_id',
    ])
  })

  it('completes columns after a table qualifier and quotes odd names', () => {
    const r = suggest('orders.', schemas, false, 'where')
    expect(r.map((s) => s.insert)).toContain('orders."Total Price"')
    expect(labels(suggest('audit.', schemas))).toEqual(['events'])
  })

  it('falls back to keywords for an empty schema and stays quiet without input', () => {
    expect(labels(suggest('sel', [], false))).toEqual(['SELECT'])
    expect(suggest('', schemas)).toEqual([])
    expect(suggest('', [], true).length).toBeGreaterThan(0)
  })
})

describe('tokenAt and previousWord', () => {
  it('finds the dotted token before the caret', () => {
    const text = 'select a from public.us'
    expect(tokenAt(text, text.length)).toEqual({
      start: 14,
      token: 'public.us',
    })
    expect(previousWord(text, 14)).toBe('from')
  })
})
