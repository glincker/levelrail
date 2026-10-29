import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  CirclePlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import {
  STATUS_LIMITS,
  reorderGroups,
  sortGroups,
} from '../lib/statusPageHelpers'
import {
  useCreateStatusGroup,
  useDeleteStatusGroup,
  useStatusGroups,
  useUpdateStatusGroup,
  type StatusGroup,
} from '../queries/statusPage'

function GroupRow({
  group,
  index,
  total,
  onMove,
  onDelete,
}: {
  group: StatusGroup
  index: number
  total: number
  onMove: (dir: -1 | 1) => void
  onDelete: () => void
}) {
  const { t } = useTranslation('statusPage')
  const update = useUpdateStatusGroup()
  const [name, setName] = useState(group.name)
  const trimmed = name.trim()
  const dirty = trimmed !== group.name && trimmed !== ''

  return (
    <li className="flex items-center gap-2 py-1.5">
      <Input
        value={name}
        maxLength={STATUS_LIMITS.groupName}
        aria-label={t('groups.rename', { name: group.name })}
        className="h-8 max-w-xs"
        onChange={(e) => {
          setName(e.target.value)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && dirty) {
            update.mutate({ ...group, name: trimmed })
          }
        }}
      />
      {dirty ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={update.isPending}
          onClick={() => {
            update.mutate({ ...group, name: trimmed })
          }}
        >
          {t('groups.save')}
        </Button>
      ) : null}
      <span className="ml-auto flex items-center gap-1">
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          disabled={index === 0}
          aria-label={t('groups.moveUp', { name: group.name })}
          onClick={() => {
            onMove(-1)
          }}
        >
          <ArrowUpGlassIcon className="size-5" aria-hidden="true" />
        </Button>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          disabled={index === total - 1}
          aria-label={t('groups.moveDown', { name: group.name })}
          onClick={() => {
            onMove(1)
          }}
        >
          <ArrowDownGlassIcon className="size-5" aria-hidden="true" />
        </Button>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t('groups.deleteLabel', { name: group.name })}
          onClick={onDelete}
        >
          <TrashGlassIcon className="size-5" aria-hidden="true" />
        </Button>
      </span>
      {update.isError ? (
        <span role="alert" className="text-xs text-destructive">
          {update.error.message}
        </span>
      ) : null}
    </li>
  )
}

export function StatusGroupsManager() {
  const { t } = useTranslation('statusPage')
  const { data } = useStatusGroups()
  const create = useCreateStatusGroup()
  const update = useUpdateStatusGroup()
  const del = useDeleteStatusGroup()
  const [name, setName] = useState('')
  const [pendingDelete, setPendingDelete] = useState<StatusGroup | null>(null)
  const groups = sortGroups(data ?? [])
  const atLimit = groups.length >= STATUS_LIMITS.groups

  return (
    <section className="space-y-2" aria-labelledby="sg-title">
      <h2 id="sg-title" className="text-sm font-semibold text-foreground">
        {t('groups.title')}
      </h2>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          const trimmed = name.trim()
          if (!trimmed || atLimit) return
          create.mutate(
            { name: trimmed, position: groups.length },
            {
              onSuccess: () => {
                setName('')
              },
            },
          )
        }}
      >
        <Input
          value={name}
          maxLength={STATUS_LIMITS.groupName}
          placeholder={t('groups.namePlaceholder')}
          aria-label={t('groups.name')}
          className="h-8 max-w-xs"
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
        <Button
          type="submit"
          size="sm"
          disabled={create.isPending || name.trim() === '' || atLimit}
        >
          <CirclePlusGlassIcon className="size-5" aria-hidden="true" />
          {t('groups.add')}
        </Button>
        {atLimit ? (
          <span className="text-xs text-muted-foreground">
            {t('groups.limit')}
          </span>
        ) : null}
      </form>
      {create.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {create.error.message}
        </p>
      ) : null}
      {groups.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('groups.empty')}</p>
      ) : (
        <ul className="divide-y divide-border">
          {groups.map((g, i) => (
            <GroupRow
              key={g.id}
              group={g}
              index={i}
              total={groups.length}
              onMove={(dir) => {
                if (!g.id) return
                for (const changed of reorderGroups(groups, g.id, dir)) {
                  update.mutate(changed)
                }
              }}
              onDelete={() => {
                setPendingDelete(g)
              }}
            />
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
        title={t('groups.confirmTitle', { name: pendingDelete?.name ?? '' })}
        description={t('groups.confirmBody')}
        confirmLabel={t('groups.delete')}
        tone="destructive"
        pending={del.isPending}
        error={del.isError ? del.error.message : null}
        onConfirm={() => {
          const id = pendingDelete?.id
          if (!id) return
          del.mutate(id, {
            onSuccess: () => {
              setPendingDelete(null)
            },
          })
        }}
      />
    </section>
  )
}
