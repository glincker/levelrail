import { describe, expect, it } from 'vitest'
import {
  confirmName,
  detectAllDestructive,
  detectDestructive,
} from './destructiveSql'

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
    ['WITH x AS (SELECT 1) DELETE FROM orders', 'deleteAll', 'orders'],
    ['with x as (select 1) delete from orders where id = 1', null],
    ['WITH x AS (SELECT 1) UPDATE orders SET a = 1', 'updateAll', 'orders'],
    [
      'with recursive a as (select (1)), b as (select 2) update t set a=1',
      'updateAll',
      't',
    ],
    ['with x as (select 1) insert into t select * from x', null],
    ['with x as (select 1) select * from x', null],
    ['DROP TABLE `orders`', 'drop', 'orders'],
    ['drop table `db`.`orders`', 'drop', 'db.orders'],
    ['TRUNCATE TABLE `orders`', 'truncate', 'orders'],
    ['DELETE FROM `db`.`orders`', 'deleteAll', 'db.orders'],
    ['UPDATE `orders` SET a = 1', 'updateAll', 'orders'],
    ['ALTER TABLE `orders` DROP COLUMN a', 'alterDrop', 'orders'],
    ['delete from "public"."orders"', 'deleteAll', 'public.orders'],
    ['/* c */ DROP TABLE x', 'drop', 'x'],
    ['-- c\nDROP TABLE x', 'drop', 'x'],
    ["SELECT 'delete from x'", null],
  ]
  it.each(cases)('%s', (sql, kind, object) => {
    const hit = detectDestructive(sql)
    expect(hit?.kind ?? null).toBe(kind)
    if (object) expect(hit?.object).toBe(object)
  })

  it('reports every destructive statement', () => {
    const all = detectAllDestructive(
      'select 1; drop table a; delete from b; update c set x=1 where id=1; truncate d',
    )
    expect(all.map((d) => d.kind)).toEqual(['drop', 'deleteAll', 'truncate'])
  })

  it('asks for the table name on drop and truncate only', () => {
    expect(detectDestructive('drop table a.b')?.typeToConfirm).toBe(true)
    expect(detectDestructive('delete from a')?.typeToConfirm).toBe(false)
    const d = detectDestructive('drop table a.b')
    expect(d && confirmName(d)).toBe('b')
  })
})
