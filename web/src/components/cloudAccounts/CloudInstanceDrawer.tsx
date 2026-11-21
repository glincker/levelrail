import { useId, useState } from 'react'
import type { CloudInstanceDetail } from '../../types/cloudAccounts'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { tagsToRows } from '../../lib/cloudInstances'
import {
  useCloudInstance,
  useSetInstanceTags,
} from '../../queries/cloudAccounts'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function TagEditor({
  accountId,
  d,
  writable,
}: Readonly<{
  accountId: string
  d: CloudInstanceDetail
  writable: boolean
}>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const save = useSetInstanceTags(accountId)
  const [draft, setDraft] = useState<[string, string][]>(() =>
    tagsToRows(d.tags),
  )

  function submit() {
    if (!d) return
    const tags: Record<string, string> = {}
    for (const [k, v] of draft) if (k.trim() !== '') tags[k.trim()] = v
    // An empty value removes a tag server side, so removed keys are sent empty.
    for (const k of Object.keys(d.tags)) if (!(k in tags)) tags[k] = ''
    if (Object.keys(tags).length === 0) return
    save.mutate({ iid: d.id, tags })
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <h3 className="text-xs font-semibold text-muted-foreground">
        {t('instances.detail.tags')}
      </h3>
      {draft.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('instances.detail.noTags')}
        </p>
      ) : null}
      {draft.map(([k, v], i) => (
        <div key={`${uid}-${String(i)}`} className="flex items-center gap-2">
          <Input
            aria-label={t('instances.detail.tagKey')}
            value={k}
            disabled={!writable || k in d.tags}
            className="font-mono"
            onChange={(e) => {
              setDraft((rows) =>
                rows.map((r, j) => (j === i ? [e.target.value, r[1]] : r)),
              )
            }}
          />
          <Input
            aria-label={t('instances.detail.tagValue')}
            value={v}
            disabled={!writable}
            className="font-mono"
            onChange={(e) => {
              setDraft((rows) =>
                rows.map((r, j) => (j === i ? [r[0], e.target.value] : r)),
              )
            }}
          />
          {writable ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setDraft((rows) => rows.filter((_, j) => j !== i))
              }}
            >
              {t('instances.detail.removeTag')}
            </Button>
          ) : null}
        </div>
      ))}
      {writable ? (
        <div className="flex items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              setDraft((rows) => [...rows, ['', '']])
            }}
          >
            {t('instances.detail.addTag')}
          </Button>
          <Button type="submit" size="sm" disabled={save.isPending}>
            {t('instances.detail.saveTags')}
          </Button>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('instances.detail.readOnly')}
        </p>
      )}
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(save.error)}
        </p>
      ) : null}
      {save.isSuccess ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('instances.detail.saved')}
        </p>
      ) : null}
    </form>
  )
}

export function CloudInstanceDrawer({
  accountId,
  instanceId,
  canEdit,
  onClose,
}: Readonly<{
  accountId: string
  instanceId: string | null
  canEdit: boolean
  onClose: () => void
}>) {
  const { t } = useTranslation('cloudAccounts')
  const detail = useCloudInstance(accountId, instanceId)
  const d = detail.data
  const writable =
    canEdit &&
    d !== undefined &&
    d.managed &&
    !d.read_only &&
    d.verbs.includes('tags')

  const facts: [string, string | undefined][] = d
    ? [
        [t('instances.detail.state'), d.state],
        [t('instances.detail.shape'), d.shape],
        [t('instances.detail.region'), d.region],
        [t('instances.detail.zone'), d.zone],
        [t('instances.detail.publicIp'), d.public_ipv4],
        [t('instances.detail.privateIp'), d.private_ipv4],
        [t('instances.detail.image'), d.image_id],
        [
          t('instances.detail.bootVolume'),
          d.boot_volume_gb ? `${String(d.boot_volume_gb)} GB` : undefined,
        ],
        [t('instances.detail.project'), d.project],
        [t('instances.detail.environment'), d.environment],
      ]
    : []

  return (
    <Sheet
      open={instanceId !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent side="right">
        <SheetHeader>
          <SheetTitle>{d?.name || instanceId}</SheetTitle>
          <SheetDescription className="font-mono">
            {instanceId}
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-4 overflow-y-auto px-4 pb-4">
          {detail.isPending ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('section.loading')}
            </p>
          ) : null}
          {detail.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {messageOf(detail.error)}
            </p>
          ) : null}
          {d ? (
            <>
              <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                {facts
                  .filter(([, v]) => v)
                  .map(([k, v]) => (
                    <div key={k}>
                      <dt className="text-xs text-muted-foreground">{k}</dt>
                      <dd className="font-mono break-all">{v}</dd>
                    </div>
                  ))}
              </dl>
              {d.resize_advisory ? (
                <p className="text-xs text-muted-foreground">
                  {d.resize_advisory}
                </p>
              ) : null}
              <TagEditor
                key={d.id}
                accountId={accountId}
                d={d}
                writable={writable}
              />
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
