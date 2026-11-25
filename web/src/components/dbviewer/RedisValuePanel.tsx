import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import type { RedisEntry, RedisValue } from '../../types/databaseViewer'

type StringView = 'text' | 'json' | 'hex'

function toHex(text: string): string {
  const bytes = new TextEncoder().encode(text)
  const rows: string[] = []
  for (let i = 0; i < bytes.length; i += 16) {
    const chunk = Array.from(bytes.slice(i, i + 16))
    const hex = chunk.map((b) => b.toString(16).padStart(2, '0')).join(' ')
    const ascii = chunk
      .map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : '.'))
      .join('')
    rows.push(`${i.toString(16).padStart(8, '0')}  ${hex.padEnd(47)}  ${ascii}`)
  }
  return rows.join('\n')
}

function tryParseJson(text: string): unknown {
  try {
    return JSON.parse(text) as unknown
  } catch {
    return undefined
  }
}

function JsonNode({ label, value }: { label?: string; value: unknown }) {
  const prefix =
    label === undefined ? null : (
      <span className="text-muted-foreground">{label}: </span>
    )
  if (value !== null && typeof value === 'object') {
    const entries: [string, unknown][] = Array.isArray(value)
      ? value.map((v, i) => [String(i), v])
      : Object.entries(value)
    return (
      <details open className="ml-3">
        <summary className="cursor-pointer font-mono text-xs">
          {prefix}
          {Array.isArray(value) ? `[${entries.length}]` : `{${entries.length}}`}
        </summary>
        {entries.map(([k, v]) => (
          <JsonNode key={k} label={k} value={v} />
        ))}
      </details>
    )
  }
  return (
    <div className="ml-3 break-all font-mono text-xs">
      {prefix}
      {JSON.stringify(value)}
    </div>
  )
}

function EntryRow({ type, e }: { type: string; e: RedisEntry }) {
  if (type === 'stream') {
    return (
      <details className="ml-1 py-0.5">
        <summary className="cursor-pointer font-mono text-xs">{e.id}</summary>
        <div className="ml-4">
          {(e.pairs ?? []).map((p, i) => (
            <div key={i} className="break-all font-mono text-xs">
              <span className="text-muted-foreground">{p.field}: </span>
              {p.value}
            </div>
          ))}
        </div>
      </details>
    )
  }
  return (
    <div className="break-all py-0.5 font-mono text-xs">
      {e.field !== undefined ? (
        <span className="text-muted-foreground">{e.field}: </span>
      ) : null}
      {e.value}
      {e.score !== undefined ? (
        <span className="ml-2 text-muted-foreground">({e.score})</span>
      ) : null}
    </div>
  )
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`
}

// Type aware, read-only value view for one key.
export function RedisValuePanel({ value }: { value: RedisValue }) {
  const { t } = useTranslation('databases')
  const [view, setView] = useState<StringView>('text')
  const text = value.lines.join('\n')
  const parsed = useMemo(
    () =>
      value.type === 'string' || value.type === 'ReJSON-RL'
        ? tryParseJson(text)
        : undefined,
    [value.type, text],
  )

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-1.5">
        <Badge variant="muted">{value.type}</Badge>
        <Badge variant="muted">
          {t('viewer.keys.ttl')}:{' '}
          {value.ttl < 0
            ? t('viewer.keys.noTtl')
            : t('viewer.keys.ttlSeconds', { count: value.ttl })}
        </Badge>
        {value.encoding ? (
          <Badge variant="muted">
            {t('viewer.keys.encoding')}: {value.encoding}
          </Badge>
        ) : null}
        {value.memoryBytes >= 0 ? (
          <Badge variant="muted">
            {t('viewer.keys.memory')}: {formatBytes(value.memoryBytes)}
          </Badge>
        ) : null}
        {value.size >= 0 ? (
          <Badge variant="muted">
            {t('viewer.keys.size')}: {value.size}
          </Badge>
        ) : null}
        {value.bitLength ? (
          <Badge variant="muted">
            {t('viewer.keys.bits', {
              set: value.bitCount ?? 0,
              total: value.bitLength,
            })}
          </Badge>
        ) : null}
      </div>

      {value.unsupported ? (
        <p className="text-xs text-muted-foreground">
          {t('viewer.keys.jsonUnsupported')}
        </p>
      ) : value.subtype === 'hyperloglog' ? (
        <p className="text-xs text-muted-foreground">
          {t('viewer.keys.hyperloglog')}
        </p>
      ) : value.type === 'ReJSON-RL' ? (
        parsed !== undefined ? (
          <div className="max-h-96 overflow-auto rounded-md border border-border p-2">
            <JsonNode value={parsed} />
          </div>
        ) : (
          <CodeBlock code={text} wrap maxHeight="lg" />
        )
      ) : value.type === 'string' ? (
        <>
          <div
            className="flex gap-1"
            role="group"
            aria-label={t('viewer.keys.view')}
          >
            {(['text', 'json', 'hex'] as const).map((v) => (
              <Button
                key={v}
                type="button"
                size="sm"
                variant={view === v ? 'default' : 'outline'}
                aria-pressed={view === v}
                onClick={() => {
                  setView(v)
                }}
              >
                {t(`viewer.keys.view_${v}`)}
              </Button>
            ))}
          </div>
          {view === 'hex' ? (
            <CodeBlock code={toHex(text)} maxHeight="lg" />
          ) : view === 'json' ? (
            parsed !== undefined ? (
              <div className="max-h-96 overflow-auto rounded-md border border-border p-2">
                <JsonNode value={parsed} />
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                {t('viewer.keys.notJson')}
              </p>
            )
          ) : (
            <CodeBlock code={text} wrap maxHeight="lg" />
          )}
        </>
      ) : value.entries.length > 0 ? (
        <div className="max-h-96 divide-y divide-border/60 overflow-auto rounded-md border border-border px-2">
          {value.entries.map((e, i) => (
            <EntryRow key={i} type={value.type} e={e} />
          ))}
        </div>
      ) : (
        <CodeBlock code={text} wrap maxHeight="lg" />
      )}

      {value.truncated ? (
        <p className="text-xs text-muted-foreground">
          {t('viewer.keys.truncated')}
        </p>
      ) : null}
    </div>
  )
}
