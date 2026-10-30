import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { StatusPill } from '@/components/kit'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { FunctionItem } from '../../queries/functions'

function formatMs(ms: number): string {
  if (ms <= 0) return '-'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms)} ms`
}

export function FunctionsTable({ items }: Readonly<{ items: FunctionItem[] }>) {
  const { t } = useTranslation('functions')
  const columns = useMemo<ResponsiveColumn<FunctionItem>[]>(
    () => [
      {
        id: 'name',
        header: t('list.name'),
        primary: true,
        cell: (fn) => (
          <span className="font-medium">
            <Link
              to="/functions/$name"
              params={{ name: fn.name }}
              className="hover:underline"
            >
              {fn.name}
            </Link>
            {fn.url ? (
              <a
                href={fn.url}
                target="_blank"
                rel="noreferrer"
                className="ml-2 inline-flex align-middle text-muted-foreground hover:text-foreground"
                aria-label={t('list.visit')}
              >
                <ExternalLinkGlassIcon className="size-5" aria-hidden="true" />
              </a>
            ) : null}
          </span>
        ),
      },
      {
        id: 'state',
        header: t('list.state'),
        cell: (fn) => (
          <StatusPill
            tone={fn.sleeping ? 'neutral' : 'success'}
            label={fn.sleeping ? t('list.asleep') : t('list.awake')}
            size="sm"
          />
        ),
      },
      {
        id: 'calls',
        header: t('list.calls'),
        align: 'end',
        cell: (fn) => (
          <span className="tabular-nums">
            {Math.round(fn.invocations).toLocaleString()}
          </span>
        ),
      },
      {
        id: 'p95',
        header: t('list.p95'),
        align: 'end',
        priority: 2,
        cell: (fn) => (
          <span className="tabular-nums">{formatMs(fn.p95_ms)}</span>
        ),
      },
      {
        id: 'errors',
        header: t('list.errors'),
        align: 'end',
        priority: 2,
        cell: (fn) => (
          <span className="tabular-nums">
            {fn.invocations > 0
              ? `${(fn.error_rate_5xx * 100).toFixed(1)}%`
              : '-'}
          </span>
        ),
      },
      {
        id: 'idle',
        header: t('list.idle'),
        priority: 3,
        cell: (fn) => t('list.idleValue', { minutes: fn.idle_minutes }),
      },
    ],
    [t],
  )

  return (
    <ResponsiveTable
      rows={items}
      columns={columns}
      getRowId={(fn) => fn.name}
      ariaLabel={t('list.name')}
    />
  )
}
