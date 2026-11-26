import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronLeftGlassIcon,
  ChevronRightGlassIcon,
  CopyGlassIcon,
  PlayGlassIcon,
  SettingsGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../../lib/apiError'
import {
  checkMongoJson,
  documentTitle,
  prettyDocument,
  type MongoJsonCheck,
  type MongoJsonKind,
} from '../../../lib/mongoExplorer'
import { useMongoFind } from '../../../queries/databaseMongo'
import type { JsonValue, MongoFindParams } from '../../../types/databaseMongo'
import { useCopy } from '../explorer/useCopy'
import { JsonTree, type ExpandSignal } from './JsonTree'
import { MongoDocSheet } from './MongoDocSheet'

const LIMIT_CHOICES = [10, 25, 50, 100, 250, 500]
const DEFAULT_LIMIT = 50
const EXAMPLES = [
  '{}',
  '{"createdAt": {"$gte": {"$date": "2026-01-01T00:00:00Z"}}}',
  '{"tags": {"$in": ["vip"]}}',
  '{"name": {"$regex": "^a", "$options": "i"}}',
]

interface Applied {
  filter?: JsonValue
  sort?: JsonValue
  projection?: JsonValue
}

function JsonField({
  id,
  kind,
  label,
  value,
  check,
  placeholder,
  onChange,
}: {
  id: string
  kind: MongoJsonKind
  label: string
  value: string
  check: MongoJsonCheck
  placeholder: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation('databases')
  const invalid = !check.ok
  return (
    <div className="min-w-0 flex-1 space-y-1">
      <label htmlFor={id} className="text-xs font-medium">
        {label}
      </label>
      <Input
        id={id}
        value={value}
        spellCheck={false}
        autoComplete="off"
        className="font-mono text-xs"
        placeholder={placeholder}
        aria-invalid={invalid}
        aria-describedby={invalid ? `${id}-err` : undefined}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      />
      {!check.ok ? (
        <p id={`${id}-err`} role="alert" className="text-xs text-tone-danger">
          {t(`viewer.mongo.docs.invalid.${check.error}`, {
            kind: t(`viewer.mongo.docs.kind.${kind}`),
          })}
        </p>
      ) : null}
    </div>
  )
}

export function MongoDocuments({
  name,
  database,
  collection,
  maxRows,
}: {
  name: string
  database: string
  collection: string
  maxRows: number
}) {
  const { t } = useTranslation('databases')
  const copy = useCopy()
  const uid = useId()
  const limits = useMemo(() => {
    const cap = maxRows > 0 ? maxRows : DEFAULT_LIMIT
    return LIMIT_CHOICES.filter((n) => n <= cap)
  }, [maxRows])
  const [limit, setLimit] = useState(() =>
    Math.min(DEFAULT_LIMIT, limits[limits.length - 1] ?? DEFAULT_LIMIT),
  )
  const [filterText, setFilterText] = useState('')
  const [sortText, setSortText] = useState('')
  const [projText, setProjText] = useState('')
  const [showOptions, setShowOptions] = useState(false)
  const [applied, setApplied] = useState<Applied>({})
  const [cursors, setCursors] = useState<JsonValue[]>([])
  const [signal, setSignal] = useState<ExpandSignal | undefined>()
  const [detail, setDetail] = useState<number | null>(null)

  const filterCheck = checkMongoJson(filterText, 'filter')
  const sortCheck = checkMongoJson(sortText, 'sort')
  const projCheck = checkMongoJson(projText, 'projection')
  const valid = filterCheck.ok && sortCheck.ok && projCheck.ok

  const params: MongoFindParams = {
    database,
    collection,
    ...applied,
    limit,
    after: cursors[cursors.length - 1],
  }
  const find = useMongoFind(name, params)
  const docs = find.data?.documents ?? []
  const nextAfter = find.data?.next_after

  function run() {
    if (!filterCheck.ok || !sortCheck.ok || !projCheck.ok) return
    setApplied({
      filter: filterCheck.value,
      sort: sortCheck.value,
      projection: projCheck.value,
    })
    setCursors([])
  }

  const err = find.error
  const rejected = err instanceof ApiError && err.status === 400

  return (
    <div className="space-y-3">
      <form
        className="space-y-2"
        onSubmit={(e) => {
          e.preventDefault()
          run()
        }}
      >
        <div className="flex flex-wrap items-end gap-2">
          <JsonField
            id={`${uid}-filter`}
            kind="filter"
            label={t('viewer.mongo.docs.filter')}
            value={filterText}
            check={filterCheck}
            placeholder={t('viewer.mongo.docs.filterPlaceholder')}
            onChange={setFilterText}
          />
          <div className="space-y-1">
            <label htmlFor={`${uid}-limit`} className="text-xs font-medium">
              {t('viewer.mongo.docs.limit')}
            </label>
            <NativeSelect
              id={`${uid}-limit`}
              value={limit}
              onChange={(e) => {
                setLimit(Number(e.target.value))
                setCursors([])
              }}
            >
              {limits.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </NativeSelect>
          </div>
          <Button type="submit" disabled={!valid || find.isFetching}>
            <PlayGlassIcon className="size-5" aria-hidden="true" />
            {t('viewer.mongo.docs.run')}
          </Button>
          <Button
            type="button"
            variant="outline"
            aria-expanded={showOptions}
            onClick={() => {
              setShowOptions(!showOptions)
            }}
          >
            <SettingsGlassIcon className="size-5" aria-hidden="true" />
            {t('viewer.mongo.docs.options')}
          </Button>
        </div>
        {showOptions ? (
          <div className="flex flex-wrap gap-2">
            <JsonField
              id={`${uid}-sort`}
              kind="sort"
              label={t('viewer.mongo.docs.sort')}
              value={sortText}
              check={sortCheck}
              placeholder={t('viewer.mongo.docs.sortPlaceholder')}
              onChange={setSortText}
            />
            <JsonField
              id={`${uid}-proj`}
              kind="projection"
              label={t('viewer.mongo.docs.projection')}
              value={projText}
              check={projCheck}
              placeholder={t('viewer.mongo.docs.projectionPlaceholder')}
              onChange={setProjText}
            />
          </div>
        ) : null}
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-muted-foreground">
            {t('viewer.mongo.docs.examples')}
          </span>
          {EXAMPLES.map((ex) => (
            <button
              key={ex}
              type="button"
              className="max-w-56 truncate rounded-full border border-border px-2 py-0.5 font-mono text-xs hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => {
                setFilterText(ex === '{}' ? '' : ex)
              }}
            >
              {ex}
            </button>
          ))}
        </div>
      </form>

      <div className="flex flex-wrap items-center gap-2 text-xs" role="status">
        <Badge
          variant="muted"
          title={t('viewer.mongo.readOnlyReason')}
          aria-label={`${t('viewer.mongo.readOnly')}: ${t('viewer.mongo.readOnlyReason')}`}
        >
          {t('viewer.mongo.readOnly')}
        </Badge>
        <span className="hidden text-muted-foreground sm:inline">
          {t('viewer.mongo.readOnlyReason')}
        </span>
        {find.data ? (
          <span className="ml-auto text-muted-foreground tabular-nums">
            {t('viewer.mongo.docs.status', {
              count: docs.length,
              ms: find.data.duration_ms,
              page: cursors.length + 1,
            })}
            {find.data.truncated ? ` ${t('viewer.mongo.docs.truncated')}` : ''}
          </span>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={docs.length === 0}
          onClick={() => {
            setSignal({
              version: (signal?.version ?? 0) + 1,
              open: !(signal?.open ?? false),
            })
          }}
        >
          {signal?.open
            ? t('viewer.mongo.docs.collapseAll')
            : t('viewer.mongo.docs.expandAll')}
        </Button>
      </div>

      {find.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : err ? (
        <div
          role="alert"
          className="space-y-1 rounded-lg border border-tone-danger-border bg-tone-danger-soft p-3 text-sm"
        >
          <p className="font-medium">
            {rejected
              ? t('viewer.mongo.docs.rejected')
              : t('viewer.mongo.docs.failed')}
          </p>
          <p className="font-mono text-xs break-all">{err.message}</p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void find.refetch()}
          >
            {t('viewer.states.retry')}
          </Button>
        </div>
      ) : docs.length === 0 ? (
        <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
          {cursors.length > 0 || applied.filter !== undefined
            ? t('viewer.mongo.docs.noMatches')
            : t('viewer.mongo.docs.empty')}
        </p>
      ) : (
        <ul className="space-y-2">
          {docs.map((doc, i) => (
            <li
              // Ids can be absent under a projection, so the position keys the row.
              key={`${cursors.length}-${i}`}
              className="rounded-lg border border-border bg-card p-2"
            >
              <div className="flex items-center gap-1">
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
                  {documentTitle(doc)}
                </span>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setDetail(i)
                  }}
                >
                  {t('viewer.mongo.docs.details')}
                </Button>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="size-7"
                  aria-label={t('viewer.mongo.docs.copyDoc')}
                  title={t('viewer.mongo.docs.copyDoc')}
                  onClick={() => {
                    copy(prettyDocument(doc), 'viewer.grid.copied_json')
                  }}
                >
                  <CopyGlassIcon className="size-5" aria-hidden="true" />
                </Button>
              </div>
              <JsonTree value={doc} signal={signal} />
            </li>
          ))}
        </ul>
      )}

      <div className="flex items-center justify-end gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={cursors.length === 0}
          onClick={() => {
            setCursors(cursors.slice(0, -1))
          }}
        >
          <ChevronLeftGlassIcon className="size-5" aria-hidden="true" />
          {t('viewer.mongo.docs.prev')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={nextAfter === undefined || find.isFetching}
          onClick={() => {
            if (nextAfter !== undefined) setCursors([...cursors, nextAfter])
          }}
        >
          {t('viewer.mongo.docs.next')}
          <ChevronRightGlassIcon className="size-5" aria-hidden="true" />
        </Button>
      </div>

      <MongoDocSheet
        docs={docs}
        index={detail}
        onIndexChange={setDetail}
        title={`${database}.${collection}`}
      />
    </div>
  )
}
