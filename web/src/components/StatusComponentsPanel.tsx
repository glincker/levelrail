import { useState } from 'react'
import { PlusCircleIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import {
  useCreateStatusComponent,
  useDeleteStatusComponent,
  useStatusComponents,
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
        <PlusCircleIcon className="size-4" aria-hidden="true" />
        Add component
      </Button>
    </form>
  )
}

export function StatusComponentsPanel() {
  const { data, isLoading, error } = useStatusComponents()
  const del = useDeleteStatusComponent()
  const components = data ?? []

  return (
    <div className="space-y-4">
      <AddComponentForm />
      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : components.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No components yet. Add the apps, domains or health checks you want on
          the page.
        </p>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Public name</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>Target (private)</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {components.map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="font-medium text-foreground">
                    {c.display_name}
                  </TableCell>
                  <TableCell>{c.kind}</TableCell>
                  <TableCell className="max-w-[18rem] truncate font-mono text-xs text-muted-foreground">
                    {c.target}
                  </TableCell>
                  <TableCell className="text-right">
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
                      <TrashIcon className="size-3.5" aria-hidden="true" />
                      Remove
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
