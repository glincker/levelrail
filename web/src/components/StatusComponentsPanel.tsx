import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CirclePlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { STATUS_LIMITS } from '../lib/statusPageHelpers'
import { StatusGroupsManager } from './StatusGroupsManager'
import {
  useCreateStatusComponent,
  useDeleteStatusComponent,
  useStatusComponents,
  useStatusGroups,
  useUpdateStatusComponent,
  type StatusComponent,
  type StatusComponentKind,
} from '../queries/statusPage'

const NO_GROUP = '__none__'

function ComponentGroupSelect({ c }: { c: StatusComponent }) {
  const { t } = useTranslation('statusPage')
  const { data: groups } = useStatusGroups()
  const update = useUpdateStatusComponent()
  return (
    <Select
      value={c.group_id || NO_GROUP}
      onValueChange={(v) => {
        update.mutate({
          ...c,
          group_id: typeof v === 'string' && v !== NO_GROUP ? v : '',
        })
      }}
    >
      <SelectTrigger
        className="w-full min-w-32"
        aria-label={t('component.groupLabel', { name: c.display_name })}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NO_GROUP}>{t('component.noGroup')}</SelectItem>
        {(groups ?? []).map((g) => (
          <SelectItem key={g.id} value={g.id ?? ''}>
            {g.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function ComponentDescriptionInput({ c }: { c: StatusComponent }) {
  const { t } = useTranslation('statusPage')
  const update = useUpdateStatusComponent()
  const [value, setValue] = useState(c.description ?? '')
  const save = () => {
    const next = value.trim()
    if (next !== (c.description ?? '')) {
      update.mutate({ ...c, description: next })
    }
  }
  return (
    <Input
      value={value}
      maxLength={STATUS_LIMITS.description}
      placeholder={t('component.descriptionPlaceholder')}
      aria-label={t('component.descriptionLabel', { name: c.display_name })}
      aria-invalid={update.isError}
      title={update.isError ? update.error.message : undefined}
      onChange={(e) => {
        setValue(e.target.value)
      }}
      onBlur={save}
      onKeyDown={(e) => {
        if (e.key === 'Enter') save()
      }}
    />
  )
}

function AddComponentForm() {
  const { t } = useTranslation('statusPage')
  const [kind, setKind] = useState<StatusComponentKind>('app')
  const [target, setTarget] = useState('')
  const [name, setName] = useState('')
  const create = useCreateStatusComponent()
  const hints: Record<StatusComponentKind, string> = {
    app: t('components.hintApp'),
    domain: t('components.hintDomain'),
    check: t('components.hintCheck'),
  }

  return (
    <form
      className="space-y-3 rounded-lg border border-border p-4"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate(
          {
            kind,
            target: target.trim(),
            display_name: name.trim(),
            position: 0,
          },
          {
            onSuccess: () => {
              setTarget('')
              setName('')
              toast.add({ title: t('components.added'), type: 'success' })
            },
          },
        )
      }}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <Field>
          <FieldLabel htmlFor="sc-kind">{t('components.kind')}</FieldLabel>
          <Select
            value={kind}
            onValueChange={(v) => {
              setKind(v as StatusComponentKind)
            }}
          >
            <SelectTrigger id="sc-kind" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="app">{t('components.kindApp')}</SelectItem>
              <SelectItem value="domain">
                {t('components.kindDomain')}
              </SelectItem>
              <SelectItem value="check">{t('components.kindCheck')}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="sc-target">{t('components.target')}</FieldLabel>
          <Input
            id="sc-target"
            value={target}
            placeholder={hints[kind]}
            onChange={(e) => {
              setTarget(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sc-name">
            {t('components.publicName')}
          </FieldLabel>
          <Input
            id="sc-name"
            value={name}
            placeholder={t('components.publicNamePlaceholder')}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
        </Field>
      </div>
      <FieldDescription>{t('components.privateNote')}</FieldDescription>
      {create.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {create.error.message}
        </p>
      ) : null}
      <Button type="submit" size="sm" disabled={create.isPending}>
        <CirclePlusGlassIcon className="size-5" aria-hidden="true" />
        {t('components.add')}
      </Button>
    </form>
  )
}

export function StatusComponentsPanel() {
  const { t } = useTranslation('common')
  const { t: tp } = useTranslation('statusPage')
  const { data, isLoading, error } = useStatusComponents()
  const del = useDeleteStatusComponent()
  const components = data ?? []
  const columns = useMemo<ResponsiveColumn<StatusComponent>[]>(
    () => [
      {
        id: 'name',
        header: tp('components.publicName'),
        primary: true,
        cell: (c) => (
          <span className="font-medium text-foreground">{c.display_name}</span>
        ),
      },
      {
        id: 'kind',
        header: tp('components.kind'),
        priority: 2,
        cell: (c) => c.kind,
      },
      {
        id: 'group',
        header: t('component.group', { ns: 'statusPage' }),
        cell: (c) => <ComponentGroupSelect c={c} />,
      },
      {
        id: 'description',
        header: t('component.description', { ns: 'statusPage' }),
        cell: (c) => <ComponentDescriptionInput c={c} />,
      },
      {
        id: 'target',
        header: tp('components.target'),
        cell: (c) => (
          <span className="block truncate font-mono text-xs text-muted-foreground">
            {c.target}
          </span>
        ),
      },
    ],
    [t, tp],
  )

  return (
    <div className="space-y-4">
      <StatusGroupsManager />
      <AddComponentForm />
      {isLoading ? (
        <TableSkeleton columnCount={4} rowCount={3} />
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : components.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {tp('components.empty')}
        </p>
      ) : (
        <ResponsiveTable
          rows={components}
          columns={columns}
          getRowId={(c) => c.id ?? `${c.kind}:${c.target}`}
          ariaLabel={t('tableLabels.statusComponents')}
          renderActions={(c) => (
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={del.isPending}
              aria-label={tp('components.removeLabel', {
                name: c.display_name,
              })}
              onClick={() => {
                if (c.id) del.mutate(c.id)
              }}
            >
              <TrashGlassIcon className="size-5" aria-hidden="true" />
              {tp('components.remove')}
            </Button>
          )}
        />
      )}
    </div>
  )
}
