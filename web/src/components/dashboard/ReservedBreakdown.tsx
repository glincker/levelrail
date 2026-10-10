import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { formatCores, formatSize } from '../../lib/format'
import type { GroupBy, GroupRow, Reservation } from '../../lib/reservedUsage'

const GROUP_OPTIONS: GroupBy[] = ['project', 'environment']

function ReservedPair({ cores, bytes }: { cores: number; bytes: number }) {
  return (
    <span className="tabular-nums">
      {formatCores(cores)} / {formatSize(bytes)}
    </span>
  )
}

export function ReservedBreakdown({
  rowsFor,
  top,
}: {
  rowsFor: (by: GroupBy) => GroupRow[]
  top: Reservation[]
}) {
  const { t } = useTranslation('dashboard')
  const [by, setBy] = useState<GroupBy>('project')
  const rows = rowsFor(by)

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <div className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-medium">
            {t('fleet.reserved.breakdown')}
          </h3>
          <div
            role="group"
            aria-label={t('fleet.reserved.groupBy')}
            className="flex gap-1"
          >
            {GROUP_OPTIONS.map((opt) => (
              <Button
                key={opt}
                type="button"
                size="sm"
                variant={by === opt ? 'secondary' : 'ghost'}
                aria-pressed={by === opt}
                className="h-6 px-2 text-xs"
                onClick={() => {
                  setBy(opt)
                }}
              >
                {t(`fleet.reserved.by.${opt}`)}
              </Button>
            ))}
          </div>
        </div>
        <table className="w-full text-sm">
          <caption className="sr-only">{t('fleet.reserved.breakdown')}</caption>
          <thead className="text-left text-xs text-muted-foreground">
            <tr>
              <th className="py-1 font-medium">
                {t(`fleet.reserved.by.${by}`)}
              </th>
              <th className="py-1 text-right font-medium">
                {t('fleet.reserved.colReserved')}
              </th>
              <th className="py-1 text-right font-medium">
                {t('fleet.reserved.colUsed')}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.key} className="border-t border-border/60">
                <td className="max-w-40 truncate py-1.5">
                  {r.key}
                  {r.unsetCount > 0 ? (
                    <span className="ml-1.5 text-xs text-muted-foreground">
                      {t('fleet.reserved.unsetInGroup', {
                        count: r.unsetCount,
                      })}
                    </span>
                  ) : null}
                </td>
                <td className="py-1.5 text-right">
                  <ReservedPair cores={r.cpuCores} bytes={r.memoryBytes} />
                </td>
                <td className="py-1.5 text-right text-muted-foreground">
                  <ReservedPair
                    cores={r.usedCpuCores}
                    bytes={r.usedMemoryBytes}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="text-xs text-muted-foreground">
          {t('fleet.reserved.pairHint')}
        </p>
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-medium">
          {t('fleet.reserved.topReservers')}
        </h3>
        {top.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('fleet.reserved.noReservers')}
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {top.map((r) => (
              <li
                key={r.key}
                className="flex items-center justify-between gap-2 py-1.5 text-sm"
              >
                {r.kind === 'app' ? (
                  <Link
                    to="/apps/$name"
                    params={{ name: r.name }}
                    className="min-w-0 flex-1 truncate font-medium hover:underline"
                  >
                    {r.name}
                  </Link>
                ) : (
                  <Link
                    to="/databases/$name"
                    params={{ name: r.name }}
                    className="min-w-0 flex-1 truncate font-medium hover:underline"
                  >
                    {r.name}
                  </Link>
                )}
                <span className="shrink-0 text-xs text-muted-foreground">
                  {t(`fleet.reserved.kind.${r.kind}`)}
                </span>
                <span className="shrink-0 text-xs">
                  <ReservedPair cores={r.cpuCores} bytes={r.memoryBytes} />
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
