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
import {
  useCreateStatusComponent,
  useDeleteStatusComponent,
  useStatusComponents,
  type StatusComponent,
  type StatusComponentKind,
} from '../queries/statusPage'

const TARGET_HINT: Record<StatusComponentKind, string> = {
  app: 'App name, e.g. web',
  domain: 'Hostname, e.g. example.com (DNS check)',
  check: 'https://example.com/health (probed every minute)',
}

function AddComponentForm() {
  const [kind, setKind] = useState<StatusComponentKind>('app')
  const [target, setTarget] = useState('')
  const [name, setName] = useState('')
  const create = useCreateStatusComponent()

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
              toast.add({ title: 'Component added.', type: 'success' })
            },
          },
        )
      }}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <Field>
          <FieldLabel htmlFor="sc-kind">Kind</FieldLabel>
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
              <SelectItem value="app">App</SelectItem>
              <SelectItem value="domain">Domain</SelectItem>
              <SelectItem value="check">HTTP check</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="sc-target">Target (private)</FieldLabel>
          <Input
            id="sc-target"
            value={target}
            placeholder={TARGET_HINT[kind]}
            onChange={(e) => {
              setTarget(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sc-name">Public name</FieldLabel>
          <Input
            id="sc-name"
            value={name}
            placeholder="Website"
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
        </Field>
      </div>
      <FieldDescription>
        Only the public name is ever shown. The target stays private.
      </FieldDescription>
      {create.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {create.error.message}
        </p>
      ) : null}
      <Button type="submit" size="sm" disabled={create.isPending}>
        <CirclePlusGlassIcon className="size-4.5" aria-hidden="true" />
        Add component
      </Button>
    </form>
  )
}

export function StatusComponentsPanel() {
  const { t } = useTranslation('common')
  const { data, isLoading, error } = useStatusComponents()
  const del = useDeleteStatusComponent()
  const components = data ?? []
  const columns = useMemo<ResponsiveColumn<StatusComponent>[]>(
    () => [
      {
        id: 'name',
        header: 'Public name',
        primary: true,
        cell: (c) => (
          <span className="font-medium text-foreground">{c.display_name}</span>
        ),
      },
      { id: 'kind', header: 'Kind', priority: 2, cell: (c) => c.kind },
      {
        id: 'target',
        header: 'Target (private)',
        cell: (c) => (
          <span className="block truncate font-mono text-xs text-muted-foreground">
            {c.target}
          </span>
        ),
      },
    ],
    [],
  )

  return (
    <div className="space-y-4">
      <AddComponentForm />
      {isLoading ? (
        <TableSkeleton columnCount={4} rowCount={3} />
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : components.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No components yet. Add the apps, domains or health checks you want on
          the page.
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
              aria-label={`Remove ${c.display_name}`}
              onClick={() => {
                if (c.id) del.mutate(c.id)
              }}
            >
              <TrashGlassIcon className="size-4" aria-hidden="true" />
              Remove
            </Button>
          )}
        />
      )}
    </div>
  )
}
