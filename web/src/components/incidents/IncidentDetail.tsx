import { NativeSelect } from '@/components/ui/native-select'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import { formatAbsoluteTime } from '@/lib/relativeTime'
import {
  fetchIncident,
  useAckIncident,
  useAddIncidentNote,
  useIncident,
  usePublishIncident,
  useResolveIncident,
  useSaveIncidentDraft,
} from '../../queries/incidents'
import type { Incident, IncidentPublicStatus } from '../../types/incident'

const STATUSES: IncidentPublicStatus[] = [
  'investigating',
  'identified',
  'monitoring',
  'resolved',
]

function Timeline({ incident }: { incident: Incident }) {
  const { t } = useTranslation('monitors')
  const entries = incident.timeline ?? []
  if (entries.length === 0) return null
  return (
    <ol
      aria-label={t('incidents.timeline')}
      className="space-y-1 border-l border-border pl-3"
    >
      {entries.map((e) => (
        <li key={`${e.created_at}-${e.body}`} className="text-xs">
          <span className="text-muted-foreground">
            {formatAbsoluteTime(e.created_at)}
            {e.author ? ` - ${e.author}` : ''}
          </span>
          {e.visibility === 'public' ? (
            <Badge variant="info" className="ml-1">
              {t('incidents.published')}
            </Badge>
          ) : null}
          <p className="break-words">{e.body}</p>
        </li>
      ))}
    </ol>
  )
}

function NoteForm({ id }: { id: string }) {
  const { t } = useTranslation('monitors')
  const add = useAddIncidentNote()
  const [body, setBody] = useState('')
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate(
      { id, body: body.trim() },
      {
        onSuccess: () => {
          setBody('')
        },
      },
    )
  }
  return (
    <form onSubmit={onSubmit} className="space-y-1">
      <div className="flex items-end gap-2">
        <div className="flex-1 space-y-1">
          <Label htmlFor="incident-note">{t('incidents.note.label')}</Label>
          <Input
            id="incident-note"
            value={body}
            onChange={(e) => {
              setBody(e.target.value)
            }}
          />
        </div>
        <Button
          type="submit"
          size="sm"
          disabled={!body.trim() || add.isPending}
        >
          {t('incidents.note.add')}
        </Button>
      </div>
      {add.error ? (
        <p role="alert" className="text-xs text-destructive">
          {add.error.message}
        </p>
      ) : null}
    </form>
  )
}

function DraftForm({
  incident,
  monitorName,
  onRefresh,
}: {
  incident: Incident
  monitorName?: string
  onRefresh: () => void
}) {
  const { t } = useTranslation('monitors')
  const save = useSaveIncidentDraft()
  const publish = usePublishIncident()
  const [title, setTitle] = useState(incident.draft?.title || incident.title)
  const [status, setStatus] = useState<IncidentPublicStatus>(incident.status)
  const [body, setBody] = useState(
    incident.draft?.body ||
      t('incidents.draft.prefillBody', {
        title: incident.title,
        monitor: monitorName ?? t('incidents.draft.unknownMonitor'),
      }),
  )
  const [confirming, setConfirming] = useState(false)
  const [stale, setStale] = useState(false)
  const [publishError, setPublishError] = useState<string | null>(null)
  const empty = !title.trim() || !body.trim()

  const saveDraft = () => {
    save.mutate(
      {
        id: incident.id,
        draft: { title: title.trim(), status, body: body.trim() },
      },
      {
        onSuccess: () => {
          toast.add({ title: t('incidents.draft.saved'), type: 'success' })
        },
      },
    )
  }
  const doPublish = async () => {
    setPublishError(null)
    // Save first so the reviewed text is what goes public, then send the fresh updated_at.
    try {
      await save.mutateAsync({
        id: incident.id,
        draft: { title: title.trim(), status, body: body.trim() },
      })
      const fresh = await fetchIncident(incident.id)
      // The save response carries no version, so confirm the fetched draft is ours before trusting its updated_at.
      if (
        fresh.draft?.title !== title.trim() ||
        fresh.draft.body !== body.trim() ||
        fresh.status !== status
      ) {
        setConfirming(false)
        toast.add({ title: t('incidents.publish.changed'), type: 'error' })
        return
      }
      await publish.mutateAsync({
        id: incident.id,
        updatedAt: fresh.updated_at,
      })
      setConfirming(false)
      toast.add({ title: t('incidents.publish.done'), type: 'success' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setConfirming(false)
        setStale(true)
        return
      }
      // Keep the dialog open so the operator sees the failure and can retry.
      setPublishError(err instanceof Error ? err.message : String(err))
    }
  }

  if (incident.published) {
    return (
      <p className="text-xs text-muted-foreground">
        {t('incidents.publish.already')}
      </p>
    )
  }
  return (
    <form
      className="space-y-2 rounded-lg border border-border bg-card p-3"
      onSubmit={(e) => {
        e.preventDefault()
        saveDraft()
      }}
    >
      <h3 className="text-sm font-medium text-muted-foreground">
        {t('incidents.draft.title')}
      </h3>
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-40 flex-1 space-y-1">
          <Label htmlFor="incident-draft-title">
            {t('incidents.draft.name')}
          </Label>
          <Input
            id="incident-draft-title"
            value={title}
            onChange={(e) => {
              setTitle(e.target.value)
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="incident-draft-status">
            {t('incidents.draft.status')}
          </Label>
          <NativeSelect
            id="incident-draft-status"
            value={status}
            onChange={(e) => {
              setStatus(
                STATUSES.find((s) => s === e.target.value) ?? 'investigating',
              )
            }}
          >
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`incidents.status.${s}`)}
              </option>
            ))}
          </NativeSelect>
        </div>
      </div>
      <div className="space-y-1">
        <Label htmlFor="incident-draft-body">{t('incidents.draft.body')}</Label>
        <Textarea
          id="incident-draft-body"
          rows={3}
          value={body}
          onChange={(e) => {
            setBody(e.target.value)
          }}
        />
      </div>
      {stale ? (
        <div
          role="alert"
          className="flex items-center gap-2 text-xs text-destructive"
        >
          {t('incidents.publish.stale')}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              setStale(false)
              onRefresh()
            }}
          >
            {t('incidents.refresh')}
          </Button>
        </div>
      ) : null}
      {save.error ? (
        <p role="alert" className="text-xs text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button
          type="submit"
          size="sm"
          variant="outline"
          disabled={save.isPending}
        >
          {t('incidents.draft.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={empty || save.isPending}
          onClick={() => {
            setConfirming(true)
          }}
        >
          {t('incidents.publish.action')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          setConfirming(open)
          if (!open) setPublishError(null)
        }}
        title={t('incidents.publish.confirmTitle')}
        description={t('incidents.publish.confirmBody')}
        consequences={[title.trim(), body.trim()]}
        confirmLabel={t('incidents.publish.confirm')}
        pendingLabel={t('incidents.publish.pending')}
        tone="destructive"
        focusCancel
        pending={publish.isPending || save.isPending}
        error={publishError}
        onConfirm={() => void doPublish()}
      />
    </form>
  )
}

export function IncidentDetail({
  id,
  monitorName,
}: {
  id: string
  monitorName?: string
}) {
  const { t } = useTranslation('monitors')
  const query = useIncident(id)
  const ack = useAckIncident()
  const resolve = useResolveIncident()
  const inc = query.data
  if (query.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {t('incidents.loadError', { message: query.error.message })}
      </p>
    )
  }
  if (!inc) return null
  const actionError = ack.error?.message ?? resolve.error?.message
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        {inc.internal_state === 'triggered' ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={ack.isPending}
            onClick={() => {
              ack.mutate(inc.id)
            }}
          >
            {t('incidents.ack')}
          </Button>
        ) : null}
        {inc.internal_state !== 'resolved' ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={resolve.isPending}
            onClick={() => {
              resolve.mutate(inc.id)
            }}
          >
            {t('incidents.resolve')}
          </Button>
        ) : null}
        {actionError ? (
          <span role="alert" className="text-xs text-destructive">
            {actionError}
          </span>
        ) : null}
      </div>
      <Timeline incident={inc} />
      <NoteForm id={inc.id} />
      <DraftForm
        key={inc.updated_at}
        incident={inc}
        monitorName={monitorName}
        onRefresh={() => void query.refetch()}
      />
    </div>
  )
}
