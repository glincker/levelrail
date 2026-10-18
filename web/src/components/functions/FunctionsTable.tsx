import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowSquareOutIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusPill } from '@/components/kit'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { FunctionItem } from '../../queries/functions'

function formatMs(ms: number): string {
  if (ms <= 0) return '-'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms)} ms`
}

export function FunctionsTable({ items }: Readonly<{ items: FunctionItem[] }>) {
  const { t } = useTranslation('functions')
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('list.name')}</TableHead>
          <TableHead>{t('list.state')}</TableHead>
          <TableHead className="text-right">{t('list.calls')}</TableHead>
          <TableHead className="text-right">{t('list.p95')}</TableHead>
          <TableHead className="text-right">{t('list.errors')}</TableHead>
          <TableHead>{t('list.idle')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((fn) => (
          <TableRow key={fn.name}>
            <TableCell className="font-medium">
              <Link
                to="/apps/$name"
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
                  <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
                </a>
              ) : null}
            </TableCell>
            <TableCell>
              <StatusPill
                tone={fn.sleeping ? 'neutral' : 'success'}
                label={fn.sleeping ? t('list.asleep') : t('list.awake')}
                size="sm"
              />
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {Math.round(fn.invocations).toLocaleString()}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {formatMs(fn.p95_ms)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {fn.invocations > 0
                ? `${(fn.error_rate_5xx * 100).toFixed(1)}%`
                : '-'}
            </TableCell>
            <TableCell>
              {t('list.idleValue', { minutes: fn.idle_minutes })}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
