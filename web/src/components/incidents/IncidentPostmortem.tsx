import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { formatAbsoluteTime } from '@/lib/relativeTime'
import {
  useDraftPostmortem,
  usePostmortem,
  usePublishPostmortem,
  useSavePostmortem,
} from '../../queries/incidentPostmortem'
import type {
  IncidentPostmortem as Postmortem,
  PostmortemEditInput,
} from '../../types/incident'

type TextField = 'summary' | 'impact' | 'root_cause' | 'resolution'
const FIELDS: TextField[] = ['summary', 'impact', 'root_cause', 'resolution']

function toInput(p: Postmortem): PostmortemEditInput {
  return {
    summary: p.summary,
    impact: p.impact,
    root_cause: p.root_cause,
    resolution: p.resolution,
    action_items: p.action_items,
  }
}

function Facts({ pm }: { pm: Postmortem }) {
  const { t } = useTranslation('monitors')
  return (
    <div className="space-y-1 text-xs">
      <p>
        {t('incidents.postmortem.window', {
          start: formatAbsoluteTime(pm.impact_start),
          end: pm.impact_end
            ? formatAbsoluteTime(pm.impact_end)
            : t('incidents.postmortem.ongoing'),
        })}
      </p>
      {pm.alerts.length > 0 ? (
        <ul
          aria-label={t('incidents.postmortem.alerts')}
          className="space-y-0.5"
        >
          {pm.alerts.map((a) => (
            <li
              key={`${a.kind}-${a.ref}-${a.down_at ?? ''}`}
              className="flex flex-wrap gap-1"
            >
              <Badge variant="muted">{a.kind}</Badge>
              <span className="break-all">{a.ref}</span>
              {a.down_at ? (
                <span className="text-muted-foreground">
                  {formatAbsoluteTime(a.down_at)}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground">
          {t('incidents.postmortem.noAlerts')}
        </p>
      )}
    </div>
  )
}

function Published({ pm }: { pm: Postmortem }) {
  const { t } = useTranslation('monitors')
  return (
    <div className="space-y-2">
      {FIELDS.filter((f) => pm[f].trim() !== '').map((f) => (
        <div key={f}>
          <h4 className="text-xs font-medium">
            {t(`incidents.postmortem.fields.${f}`)}
          </h4>
          <p className="whitespace-pre-wrap break-words text-sm">{pm[f]}</p>
        </div>
      ))}
      {pm.action_items.length > 0 ? (
        <div>
          <h4 className="text-xs font-medium">
            {t('incidents.postmortem.fields.action_items')}
          </h4>
          <ul className="list-disc pl-4 text-sm">
            {pm.action_items.map((it) => (
              <li key={it} className="break-words">
                {it}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {pm.published_at ? (
        <p className="text-xs text-muted-foreground">
          {t('incidents.postmortem.publishedBy', {
            at: formatAbsoluteTime(pm.published_at),
            by: pm.published_by ?? '',
          })}
        </p>
      ) : null}
    </div>
  )
}

function Editor({ pm }: { pm: Postmortem }) {
  const { t } = useTranslation('monitors')
  const save = useSavePostmortem(pm.incident_id)
  const redraft = useDraftPostmortem(pm.incident_id)
  const publish = usePublishPostmortem(pm.incident_id)
  const [form, setForm] = useState<PostmortemEditInput>(() => toInput(pm))
  const [items, setItems] = useState(pm.action_items.join('\n'))
  const [confirming, setConfirming] = useState(false)
  const current = (): PostmortemEditInput => ({
    ...form,
    action_items: items
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s !== ''),
  })
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(current())
  }
  const ready = form.summary.trim() !== '' && form.root_cause.trim() !== ''
  const error =
    save.error?.message ?? redraft.error?.message ?? publish.error?.message
  return (
    <form onSubmit={onSubmit} className="space-y-2">
      {FIELDS.map((f) => (
        <div key={f} className="space-y-1">
          <Label htmlFor={`pm-${f}`}>
            {t(`incidents.postmortem.fields.${f}`)}
          </Label>
          <Textarea
            id={`pm-${f}`}
            rows={f === 'summary' ? 3 : 2}
            value={form[f]}
            onChange={(e) => {
              setForm({ ...form, [f]: e.target.value })
            }}
          />
        </div>
      ))}
      <div className="space-y-1">
        <Label htmlFor="pm-action-items">
          {t('incidents.postmortem.fields.action_items')}
        </Label>
        <Textarea
          id="pm-action-items"
          rows={3}
          placeholder={t('incidents.postmortem.itemsHint')}
          value={items}
          onChange={(e) => {
            setItems(e.target.value)
          }}
        />
      </div>
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      <div className="flex flex-wrap justify-end gap-2">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={redraft.isPending}
          onClick={() => {
            redraft.mutate()
          }}
        >
          {t('incidents.postmortem.refresh')}
        </Button>
        <Button
          type="submit"
          size="sm"
          variant="outline"
          disabled={save.isPending}
        >
          {t('incidents.postmortem.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={!ready || save.isPending}
          onClick={() => {
            setConfirming(true)
          }}
        >
          {t('incidents.postmortem.publish')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('incidents.postmortem.confirmTitle')}
        description={t('incidents.postmortem.confirmBody')}
        confirmLabel={t('incidents.postmortem.publish')}
        pendingLabel={t('incidents.postmortem.pending')}
        focusCancel
        pending={publish.isPending || save.isPending}
        error={publish.error?.message ?? null}
        onConfirm={() => {
          save.mutate(current(), {
            onSuccess: () => {
              publish.mutate(undefined, {
                onSuccess: () => {
                  setConfirming(false)
                },
              })
            },
          })
        }}
      />
    </form>
  )
}

// Shown for resolved incidents only; every text field renders as plain text.
export function IncidentPostmortem({ id }: { id: string }) {
  const { t } = useTranslation('monitors')
  const query = usePostmortem(id)
  const draft = useDraftPostmortem(id)
  const pm = query.data
  return (
    <section
      aria-labelledby={`pm-title-${id}`}
      className="space-y-2 rounded-md border border-border p-3"
    >
      <div className="flex items-center gap-2">
        <h3 id={`pm-title-${id}`} className="text-sm font-medium">
          {t('incidents.postmortem.title')}
        </h3>
        {pm ? (
          <Badge variant={pm.state === 'published' ? 'info' : 'muted'}>
            {t(`incidents.postmortem.state.${pm.state}`)}
          </Badge>
        ) : null}
      </div>
      {query.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {query.error.message}
        </p>
      ) : null}
      {query.isSuccess && pm === null ? (
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs text-muted-foreground">
            {t('incidents.postmortem.none')}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={draft.isPending}
            onClick={() => {
              draft.mutate()
            }}
          >
            {t('incidents.postmortem.start')}
          </Button>
          {draft.error ? (
            <span role="alert" className="text-xs text-destructive">
              {draft.error.message}
            </span>
          ) : null}
        </div>
      ) : null}
      {pm ? <Facts pm={pm} /> : null}
      {pm?.state === 'published' ? <Published pm={pm} /> : null}
      {pm?.state === 'draft' ? <Editor key={pm.updated_at} pm={pm} /> : null}
    </section>
  )
}
