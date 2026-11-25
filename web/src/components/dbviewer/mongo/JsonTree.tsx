import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, ChevronRightGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { ejsonScalar, isContainer } from '../../../lib/mongoExplorer'
import type { JsonValue } from '../../../types/databaseMongo'

// Bumping version forces every node open or closed (expand all).
export interface ExpandSignal {
  version: number
  open: boolean
}

const AUTO_OPEN_DEPTH = 1

function Scalar({ value }: { value: JsonValue }) {
  const ejson = ejsonScalar(value)
  if (ejson) {
    return (
      <span className="text-tone-accent">
        {ejson.label}
        <span className="text-muted-foreground">(</span>
        {ejson.text}
        <span className="text-muted-foreground">)</span>
      </span>
    )
  }
  if (value === null) return <span className="text-muted-foreground">null</span>
  if (typeof value === 'string') {
    return <span className="text-tone-success">{JSON.stringify(value)}</span>
  }
  if (typeof value === 'number') {
    return <span className="text-tone-info">{value}</span>
  }
  return <span className="text-tone-warning">{JSON.stringify(value)}</span>
}

function Node({
  name,
  value,
  depth,
  signal,
}: {
  name: string | null
  value: JsonValue
  depth: number
  signal: ExpandSignal | undefined
}) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(
    signal ? signal.open : depth < AUTO_OPEN_DEPTH,
  )
  // Adjust during render so a new signal never costs an extra paint.
  const [seen, setSeen] = useState(signal?.version)
  if (signal && signal.version !== seen) {
    setSeen(signal.version)
    setOpen(signal.open)
  }

  const key =
    name === null ? null : <span className="text-foreground">{name}: </span>
  if (!isContainer(value)) {
    return (
      <li className="py-px pl-5">
        {key}
        <Scalar value={value} />
      </li>
    )
  }
  const isArray = Array.isArray(value)
  const entries: [string, JsonValue][] = isArray
    ? value.map((v, i) => [String(i), v])
    : Object.entries(value)
  const openBrace = isArray ? '[' : '{'
  const closeBrace = isArray ? ']' : '}'
  const summary = t(
    isArray ? 'viewer.mongo.tree.items' : 'viewer.mongo.tree.fields',
    { count: entries.length },
  )
  return (
    <li className="py-px">
      <button
        type="button"
        aria-expanded={open}
        aria-label={`${name ?? t('viewer.mongo.tree.root')}: ${summary}`}
        onClick={() => {
          setOpen(!open)
        }}
        className="flex items-center gap-0.5 rounded-sm text-left hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        {open ? (
          <ChevronDownGlassIcon
            className="size-4 shrink-0"
            aria-hidden="true"
          />
        ) : (
          <ChevronRightGlassIcon
            className="size-4 shrink-0"
            aria-hidden="true"
          />
        )}
        <span>
          {key}
          <span className="text-muted-foreground">
            {openBrace}
            {open ? '' : ` ${summary} ${closeBrace}`}
          </span>
        </span>
      </button>
      {open ? (
        <>
          <ul className="ml-2 border-l border-border pl-2">
            {entries.map(([k, v]) => (
              <Node
                key={k}
                name={isArray ? null : k}
                value={v}
                depth={depth + 1}
                signal={signal}
              />
            ))}
          </ul>
          <span className="pl-4 text-muted-foreground">{closeBrace}</span>
        </>
      ) : null}
    </li>
  )
}

export function JsonTree({
  value,
  signal,
  className,
}: {
  value: JsonValue
  signal?: ExpandSignal
  className?: string
}) {
  return (
    <ul className={cn('font-mono text-xs leading-5', className)}>
      <Node name={null} value={value} depth={0} signal={signal} />
    </ul>
  )
}
