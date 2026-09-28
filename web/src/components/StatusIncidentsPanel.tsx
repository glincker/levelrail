import { useState } from 'react'
import { PlusCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  INCIDENT_STATUSES,
  useCreateStatusIncident,
  useDeleteStatusIncident,
  usePostStatusUpdate,
  useStatusComponents,
  useStatusIncidents,
  type IncidentImpact,
  type IncidentKind,
  type StatusIncident,
} from '../queries/statusPage'

const IMPACTS: IncidentImpact[] = ['none', 'minor', 'major', 'critical']

function CreateIncidentForm() {
  const [kind, setKind] = useState<IncidentKind>('incident')
  const [title, setTitle] = useState('')
  const [impact, setImpact] = useState<IncidentImpact>('minor')
  const [body, setBody] = useState('')
  const [starts, setStarts] = useState('')
  const [ends, setEnds] = useState('')
  const [picked, setPicked] = useState<string[]>([])
  const { data: components } = useStatusComponents()
  const create = useCreateStatusIncident()

  function reset() {
    setTitle('')
    setBody('')
    setStarts('')
    setEnds('')
    setPicked([])
  }

  return (
    <form
      className="space-y-3 rounded-lg border border-border p-4"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate(
          {
            kind,
            title: title.trim(),
            status: '',
            impact: kind === 'maintenance' ? 'none' : impact,
            component_ids: picked,
            body: body.trim() || undefined,
            starts_at: starts ? new Date(starts).toISOString() : undefined,
            ends_at: ends ? new Date(ends).toISOString() : undefined,
          },
          {
            onSuccess: () => {
              reset()
              toast.add({ title: 'Announcement published.', type: 'success' })
            },
          },
        )
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="si-kind">Type</FieldLabel>
          <Select
            value={kind}
            onValueChange={(v) => {
              setKind(v as IncidentKind)
            }}
          >
            <SelectTrigger id="si-kind" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="incident">Incident</SelectItem>
              <SelectItem value="maintenance">Scheduled maintenance</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        {kind === 'incident' ? (
          <Field>
            <FieldLabel htmlFor="si-impact">Impact on components</FieldLabel>
            <Select
              value={impact}
              onValueChange={(v) => {
                setImpact(v as IncidentImpact)
              }}
            >
              <SelectTrigger id="si-impact" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {IMPACTS.map((i) => (
                  <SelectItem key={i} value={i}>
                    {i}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="si-starts">Starts</FieldLabel>
              <Input
                id="si-starts"
                type="datetime-local"
                value={starts}
                onChange={(e) => {
                  setStarts(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="si-ends">Ends</FieldLabel>
              <Input
                id="si-ends"
                type="datetime-local"
                value={ends}
                onChange={(e) => {
                  setEnds(e.target.value)
                }}
              />
            </Field>
          </div>
        )}
      </div>
      <Field>
        <FieldLabel htmlFor="si-title">Title</FieldLabel>
        <Input
          id="si-title"
          value={title}
          onChange={(e) => {
            setTitle(e.target.value)
          }}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="si-body">Message</FieldLabel>
        <Textarea
          id="si-body"
          rows={3}
          value={body}
          onChange={(e) => {
            setBody(e.target.value)
          }}
        />
        <FieldDescription>
          Supports **bold**, `code`, - lists and [links](https://example.com).
          Everything else is shown as plain text.
        </FieldDescription>
      </Field>
      {components && components.length > 0 ? (
        <fieldset className="flex flex-wrap gap-4">
          <legend className="mb-1 text-sm font-medium">
            Affected components
          </legend>
          {components.map((c) => (
            <label key={c.id} className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={c.id !== undefined && picked.includes(c.id)}
                onCheckedChange={(checked) => {
                  const id = c.id
                  if (!id) return
                  setPicked((cur) =>
                    checked ? [...cur, id] : cur.filter((x) => x !== id),
                  )
                }}
              />
              {c.display_name}
            </label>
          ))}
        </fieldset>
      ) : null}
      {create.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {create.error.message}
        </p>
      ) : null}
      <Button type="submit" size="sm" disabled={create.isPending}>
        <PlusCircleIcon className="size-4" aria-hidden="true" />
        Publish
      </Button>
    </form>
  )
}

function IncidentCard({ incident }: { incident: StatusIncident }) {
  const [status, setStatus] = useState(
    INCIDENT_STATUSES[incident.kind][
      Math.min(1, INCIDENT_STATUSES[incident.kind].length - 1)
    ] ?? '',
  )
  const [body, setBody] = useState('')
  const post = usePostStatusUpdate()
  const del = useDeleteStatusIncident()
  const closed =
    incident.status === 'resolved' || incident.status === 'completed'

  return (
    <article className="space-y-2 rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-foreground">
          {incident.title}
        </h3>
        <div className="flex items-center gap-2">
          <Badge variant="outline">{incident.kind}</Badge>
          <Badge variant={closed ? 'success' : 'warning'}>
            {incident.status.replace('_', ' ')}
          </Badge>
        </div>
      </div>
      <ul className="space-y-1 text-xs text-muted-foreground">
        {(incident.updates ?? []).map((u, i) => (
          <li key={i}>
            <span className="font-medium">{u.status.replace('_', ' ')}</span>
            {u.created_at
              ? ` (${new Date(u.created_at).toLocaleString()})`
              : ''}
            : {u.body}
          </li>
        ))}
      </ul>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (!incident.id) return
          post.mutate(
            { id: incident.id, update: { status, body: body.trim() } },
            {
              onSuccess: () => {
                setBody('')
                toast.add({ title: 'Update posted.', type: 'success' })
              },
            },
          )
        }}
      >
        <Select
          value={status}
          onValueChange={(v) => {
            if (v) setStatus(v)
          }}
        >
          <SelectTrigger className="w-40" aria-label="Update status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {INCIDENT_STATUSES[incident.kind].map((s) => (
              <SelectItem key={s} value={s}>
                {s.replace('_', ' ')}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          className="min-w-48 flex-1"
          aria-label="Update message"
          placeholder="Post an update"
          value={body}
          onChange={(e) => {
            setBody(e.target.value)
          }}
        />
        <Button
          type="submit"
          size="sm"
          disabled={post.isPending || !body.trim()}
        >
          Post update
        </Button>
        <Button
          type="button"
          variant="destructive"
          size="sm"
          disabled={del.isPending}
          onClick={() => {
            if (incident.id) del.mutate(incident.id)
          }}
        >
          Delete
        </Button>
      </form>
      {post.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {post.error.message}
        </p>
      ) : null}
    </article>
  )
}

export function StatusIncidentsPanel() {
  const { data, isLoading, error } = useStatusIncidents()
  return (
    <div className="space-y-4">
      <CreateIncidentForm />
      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : (data ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">No announcements yet.</p>
      ) : (
        (data ?? []).map((i) => <IncidentCard key={i.id} incident={i} />)
      )}
    </div>
  )
}
