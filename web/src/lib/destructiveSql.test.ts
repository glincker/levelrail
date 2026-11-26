import { describe, expect, it } from 'vitest'
import { confirmName, detectDestructive } from './destructiveSql'

describe('detectDestructive', () => {
  const cases: [string, string | null, string?][] = [
    ['select * from users', null],
    ['DROP TABLE public.users', 'drop', 'public.users'],
    ['drop table if exists "Orders"', 'drop', 'Orders'],
    ['TRUNCATE users', 'truncate', 'users'],
    ['delete from users', 'deleteAll', 'users'],
    ['delete from users where id = 1', null],
    ['update users set a = 1', 'updateAll', 'users'],
    ['update users set a = 1 where id = 2', null],
    ['alter table users drop column a', 'alterDrop', 'users'],
    ['alter table users add column a int', null],
    ['grant select on users to bob', 'grant', 'users'],
    ['revoke all on users from bob', 'revoke', 'users'],
    ["select 'drop table x'", null],
    ['-- drop table x\nselect 1', null],
    ['select 1; delete from t', 'deleteAll', 't'],
  ]
  it.each(cases)('%s', (sql, kind, object) => {
    const hit = detectDestructive(sql)
    expect(hit?.kind ?? null).toBe(kind)
    if (object) expect(hit?.object).toBe(object)
  })

  it('asks for the table name on drop and truncate only', () => {
    expect(detectDestructive('drop table a.b')?.typeToConfirm).toBe(true)
    expect(detectDestructive('delete from a')?.typeToConfirm).toBe(false)
    const d = detectDestructive('drop table a.b')
    expect(d && confirmName(d)).toBe('b')
  })
})
