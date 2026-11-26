import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  attrType,
  cellText,
  gridColumns,
  plainMap,
  typedKey,
} from '../../../lib/dynamoExplorer'
import { useDynamoRead } from '../../../queries/databaseDynamo'
import type {
  DynamoItem,
  DynamoKeyPart,
  DynamoReadParams,
  DynamoSortOp,
  DynamoTable,
} from '../../../types/databaseDynamo'
import { JsonTree } from '../mongo/JsonTree'

const LIMIT_CHOICES = [10, 25, 50, 100]
const SORT_OPS: DynamoSortOp[] = ['=', '<', '<=', '>', '>=', 'begins_with']

function keyType(table: DynamoTable, attr: string): string {
  return (
    table.AttributeDefinitions?.find((d) => d.AttributeName === attr)
      ?.AttributeType ?? 'S'
  )
}

export function DynamoItems({
  name,
  table,
}: {
  name: string
  table: DynamoTable
}) {
  const { t } = useTranslation('databases')
  const [mode, setMode] = useState<'query' | 'scan'>('query')
  const [index, setIndex] = useState('')
  const [pk, setPk] = useState('')
  const [sortOp, setSortOp] = useState<DynamoSortOp>('=')
  const [sk, setSk] = useState('')
  const [limit, setLimit] = useState(25)
  const [confirm, setConfirm] = useState(false)
  const [applied, setApplied] = useState<DynamoReadParams | null>(null)
  // Start keys of the pages before the current one, so Previous needs no engine call.
  const [trail, setTrail] = useState<(DynamoItem | undefined)[]>([])
  const [start, setStart] = useState<DynamoItem | undefined>(undefined)
  const [json, setJson] = useState<DynamoItem | null>(null)

  const schema: DynamoKeyPart[] =
    index === ''
      ? table.KeySchema
      : ([
          ...(table.GlobalSecondaryIndexes ?? []),
          ...(table.LocalSecondaryIndexes ?? []),
        ].find((i) => i.IndexName === index)?.KeySchema ?? table.KeySchema)
  const hash = schema.find((k) => k.KeyType === 'HASH')
  const range = schema.find((k) => k.KeyType === 'RANGE')
  const indexNames = [
    ...(table.GlobalSecondaryIndexes ?? []),
    ...(table.LocalSecondaryIndexes ?? []),
  ].map((i) => i.IndexName)

  const params = useMemo(
    () => (applied ? { ...applied, exclusive_start_key: start } : null),
    [applied, start],
  )
  const read = useDynamoRead(name, params)

  const pkAttr = hash ? typedKey(keyType(table, hash.AttributeName), pk) : null
  const skAttr = range
    ? typedKey(keyType(table, range.AttributeName), sk)
    : null
  const canRun =
    mode === 'scan' ? confirm : hash !== undefined && pkAttr !== null

  function run() {
    setTrail([])
    setStart(undefined)
    if (mode === 'scan') {
      setApplied({ op: 'scan', table: table.TableName, limit, confirm: true })
      return
    }
    if (!hash || !pkAttr) return
    setApplied({
      op: 'query',
      table: table.TableName,
      index: index || undefined,
      key_name: hash.AttributeName,
      key_value: pkAttr,
      sort:
        range && skAttr
          ? { name: range.AttributeName, op: sortOp, value: skAttr }
          : undefined,
      limit,
    })
  }

  const data = read.data
  const keyNames = table.KeySchema.map((k) => k.AttributeName)
  const columns = data ? gridColumns(data.items, keyNames) : []

  return (
    <div className="min-w-0 space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="truncate font-mono text-sm font-medium">
          {table.TableName}
        </h2>
        <Badge variant="outline">{t('viewer.dynamo.readOnly')}</Badge>
        {table.TimeToLive?.TimeToLiveStatus === 'ENABLED' ? (
          <Badge variant="outline">
            {t('viewer.dynamo.ttl', { attr: table.TimeToLive.AttributeName })}
          </Badge>
        ) : null}
        <span className="text-xs text-muted-foreground">
          {schema.map((k) => `${k.KeyType}: ${k.AttributeName}`).join(', ')}
        </span>
      </div>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (canRun) run()
        }}
      >
        <label className="space-y-1 text-xs">
          <span className="block text-muted-foreground">
            {t('viewer.dynamo.mode')}
          </span>
          <NativeSelect
            value={mode}
            onChange={(e) => {
              setMode(e.target.value === 'scan' ? 'scan' : 'query')
              setConfirm(false)
            }}
          >
            <option value="query">{t('viewer.dynamo.query')}</option>
            <option value="scan">{t('viewer.dynamo.scan')}</option>
          </NativeSelect>
        </label>
        {mode === 'query' ? (
          <>
            {indexNames.length > 0 ? (
              <label className="space-y-1 text-xs">
                <span className="block text-muted-foreground">
                  {t('viewer.dynamo.index')}
                </span>
                <NativeSelect
                  value={index}
                  onChange={(e) => {
                    setIndex(e.target.value)
                    setPk('')
                    setSk('')
                  }}
                >
                  <option value="">{t('viewer.dynamo.tableKey')}</option>
                  {indexNames.map((n) => (
                    <option key={n} value={n}>
                      {n}
                    </option>
                  ))}
                </NativeSelect>
              </label>
            ) : null}
            <label className="space-y-1 text-xs">
              <span className="block text-muted-foreground">
                {hash?.AttributeName ?? t('viewer.dynamo.partitionKey')}
              </span>
              <Input
                value={pk}
                className="w-44 font-mono"
                onChange={(e) => {
                  setPk(e.target.value)
                }}
              />
            </label>
            {range ? (
              <>
                <label className="space-y-1 text-xs">
                  <span className="block text-muted-foreground">
                    {range.AttributeName}
                  </span>
                  <NativeSelect
                    value={sortOp}
                    onChange={(e) => {
                      setSortOp(
                        SORT_OPS.find((o) => o === e.target.value) ?? '=',
                      )
                    }}
                  >
                    {SORT_OPS.map((o) => (
                      <option key={o} value={o}>
                        {o}
                      </option>
                    ))}
                  </NativeSelect>
                </label>
                <Input
                  value={sk}
                  aria-label={range.AttributeName}
                  className="w-40 font-mono"
                  onChange={(e) => {
                    setSk(e.target.value)
                  }}
                />
              </>
            ) : null}
          </>
        ) : (
          <label className="flex items-center gap-2 pb-2 text-xs">
            <input
              type="checkbox"
              checked={confirm}
              onChange={(e) => {
                setConfirm(e.target.checked)
              }}
            />
            {t('viewer.dynamo.scanConfirm')}
          </label>
        )}
        <label className="space-y-1 text-xs">
          <span className="block text-muted-foreground">
            {t('viewer.dynamo.limit')}
          </span>
          <NativeSelect
            value={limit}
            onChange={(e) => {
              setLimit(Number(e.target.value))
            }}
          >
            {LIMIT_CHOICES.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </NativeSelect>
        </label>
        <Button type="submit" disabled={!canRun || read.isFetching}>
          {t('viewer.dynamo.run')}
        </Button>
      </form>
      {read.isError ? (
        <p role="alert" className="text-sm text-tone-danger">
          {read.error.message}
        </p>
      ) : null}
      {read.isPending && applied ? <Skeleton className="h-32 w-full" /> : null}
      {data ? (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            {t('viewer.dynamo.summary', {
              count: data.count,
              scanned: data.scanned_count,
              ms: data.duration_ms,
            })}
          </p>
          {data.items.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              {t('viewer.dynamo.noItems')}
            </p>
          ) : (
            <div className="overflow-auto rounded-lg border border-border">
              <table className="w-full text-left text-sm">
                <thead className="bg-muted text-xs">
                  <tr>
                    {columns.map((c) => (
                      <th key={c} className="px-2 py-1 font-medium">
                        {c}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((it, i) => (
                    <tr
                      key={i}
                      tabIndex={0}
                      className="cursor-pointer border-t border-border hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                      onClick={() => {
                        setJson(it)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') setJson(it)
                      }}
                    >
                      {columns.map((c) => (
                        <td
                          key={c}
                          title={attrType(it[c])}
                          className="max-w-64 truncate px-2 py-1 font-mono text-xs"
                        >
                          {cellText(it[c])}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className="flex items-center gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={trail.length === 0 || read.isFetching}
              onClick={() => {
                setStart(trail[trail.length - 1])
                setTrail(trail.slice(0, -1))
              }}
            >
              {t('viewer.dynamo.prev')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={!data.last_evaluated_key || read.isFetching}
              onClick={() => {
                setTrail([...trail, start])
                setStart(data.last_evaluated_key)
              }}
            >
              {t('viewer.dynamo.next')}
            </Button>
          </div>
        </div>
      ) : null}
      {json ? (
        <div className="space-y-2 rounded-lg border border-border bg-card p-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">{t('viewer.dynamo.item')}</h3>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setJson(null)
              }}
            >
              {t('viewer.dynamo.close')}
            </Button>
          </div>
          <JsonTree value={plainMap(json)} />
        </div>
      ) : null}
    </div>
  )
}
