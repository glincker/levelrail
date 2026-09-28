import { useState } from 'react'
import { ArrowSquareOutIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useSaveStatusPageSettings,
  useStatusPageSettings,
  useStatusPreview,
  type StatusPageSettings,
} from '../queries/statusPage'

const STATUS_VARIANT = {
  operational: 'success',
  degraded: 'warning',
  outage: 'destructive',
  maintenance: 'outline',
  unknown: 'muted',
} as const

function SettingsForm({ initial }: { initial: StatusPageSettings }) {
  const [enabled, setEnabled] = useState(initial.enabled)
  const [title, setTitle] = useState(initial.title)
  const [description, setDescription] = useState(initial.description)
  const [domain, setDomain] = useState(initial.custom_domain)
  const save = useSaveStatusPageSettings()

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(
          { enabled, title, description, custom_domain: domain },
          {
            onSuccess: () => {
              toast.add({ title: 'Status page saved.', type: 'success' })
            },
          },
        )
      }}
    >
      <label className="flex items-center gap-3 text-sm font-medium">
        <Switch checked={enabled} onCheckedChange={setEnabled} />
        Publish the status page
      </label>
      <Field>
        <FieldLabel htmlFor="sp-title">Title</FieldLabel>
        <Input
          id="sp-title"
          value={title}
          placeholder="Service status"
          onChange={(e) => {
            setTitle(e.target.value)
          }}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="sp-desc">Description</FieldLabel>
        <Input
          id="sp-desc"
          value={description}
          onChange={(e) => {
            setDescription(e.target.value)
          }}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="sp-domain">Custom domain</FieldLabel>
        <Input
          id="sp-domain"
          value={domain}
          placeholder="status.example.com"
          onChange={(e) => {
            setDomain(e.target.value)
          }}
        />
        <FieldDescription>
          Optional. Point the domain at this control plane through your ingress;
          on that host only the status page is served.
        </FieldDescription>
      </Field>
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <Button type="submit" disabled={save.isPending}>
        {save.isPending ? 'Saving...' : 'Save'}
      </Button>
    </form>
  )
}

function PreviewCard() {
  const { data, isError } = useStatusPreview()
  if (isError || !data) return null
  return (
    <section
      className="space-y-2 rounded-lg border border-border p-4"
      aria-label="What the public sees"
    >
      <h2 className="text-sm font-semibold text-foreground">
        What the public sees
      </h2>
      <p className="text-sm text-muted-foreground">
        {data.title}: {data.status_text}
      </p>
      {data.components.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          No components yet. Add some on the Components tab.
        </p>
      ) : (
        <ul className="divide-y divide-border text-sm">
          {data.components.map((c) => (
            <li
              key={c.name}
              className="flex items-center justify-between gap-3 py-1.5"
            >
              <span>{c.name}</span>
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                {c.uptime_90d === null
                  ? 'no data'
                  : `${c.uptime_90d.toFixed(2)}% uptime`}
                <Badge
                  variant={
                    STATUS_VARIANT[c.status as keyof typeof STATUS_VARIANT] ??
                    'muted'
                  }
                >
                  {c.status}
                </Badge>
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function StatusPageSettingsPanel() {
  const { data, isLoading, error } = useStatusPageSettings()
  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading...</p>
  }
  if (error || !data) {
    return <p className="text-sm text-destructive">{error?.message}</p>
  }
  return (
    <div className="space-y-6">
      {data.enabled ? (
        <a
          href={data.public_path ?? '/public/status'}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1.5 text-sm text-primary underline underline-offset-2"
        >
          Open the public page
          <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
        </a>
      ) : (
        <p className="text-sm text-muted-foreground">
          The page is off. Nothing is published until you switch it on.
        </p>
      )}
      <SettingsForm key={data.title + String(data.enabled)} initial={data} />
      <PreviewCard />
    </div>
  )
}
