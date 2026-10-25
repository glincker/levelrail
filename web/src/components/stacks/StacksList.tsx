import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ResponsiveTable,
  ROW_LINK_OVERLAY,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { RelativeTime } from '@/components/kit'
import type { Stack } from '../../types/stacks'
import { RevisionStatusBadge, StackFlags } from './StackIndicators'

export function StacksList({
  stacks,
  onOpen,
}: Readonly<{ stacks: readonly Stack[]; onOpen: (name: string) => void }>) {
  const { t } = useTranslation('stacks')
  const columns = useMemo<ResponsiveColumn<Stack>[]>(
    () => [
      {
        id: 'name',
        header: t('columns.name'),
        primary: true,
        cell: (s) => (
          <button
            type="button"
            className={`truncate text-left text-sm font-medium text-foreground ${ROW_LINK_OVERLAY}`}
            onClick={() => {
              onOpen(s.name)
            }}
          >
            {s.name}
          </button>
        ),
      },
      {
        id: 'project',
        header: t('columns.project'),
        priority: 2,
        cell: (s) => <span className="text-muted-foreground">{s.project}</span>,
      },
      {
        id: 'revision',
        header: t('columns.revision'),
        cell: (s) => (
          <span className="font-mono text-xs">
            {s.latest_revision > 0
              ? t('detail.revisionShort', { number: s.latest_revision })
              : '-'}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('columns.status'),
        cell: (s) => <RevisionStatusBadge status={s.latest_status} />,
      },
      {
        id: 'flags',
        header: t('columns.flags'),
        priority: 2,
        cell: (s) => <StackFlags stack={s} />,
      },
      {
        id: 'created',
        header: t('columns.created'),
        priority: 3,
        cell: (s) => <RelativeTime at={s.created_at} />,
      },
    ],
    [t, onOpen],
  )
  return (
    <ResponsiveTable
      rows={stacks}
      columns={columns}
      getRowId={(s) => s.name}
      ariaLabel={t('title')}
    />
  )
}
