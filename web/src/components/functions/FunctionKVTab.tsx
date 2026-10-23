import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  fetchKVValue,
  useDeleteKV,
  useFunctionKV,
  usePutKV,
  useRotateKVToken,
  type KVKey,
} from '../../queries/functionKv'

interface Draft {
  key: string
  value: string
  ttl: string
  isNew: boolean
}

const EMPTY_DRAFT: Draft = { key: '', value: '', ttl: '', isNew: true }

function EditorForm({
  name,
  draft,
  onDone,
}: Readonly<{ name: string; draft: Draft; onDone: () => void }>) {
  const { t } = useTranslation('functions')
  const put = usePutKV(name)
  const [key, setKey] = useState(draft.key)
  const [value, setValue] = useState(draft.value)
  const [ttl, setTtl] = useState(draft.ttl)
  return (
    <form
      noValidate
      className="space-y-3 rounded-lg border border-input p-4"
      onSubmit={(e) => {
        e.preventDefault()
        put.mutate(
          { key, value, ttl_seconds: Number(ttl) || 0 },
          {
            onSuccess: () => {
              toast.add({ title: t('kv.saved'), type: 'success' })
              onDone()
            },
          },
        )
      }}
    >
      <div className="grid gap-3 sm:grid-cols-[1fr_10rem]">
        <div className="space-y-1.5">
          <Label htmlFor="kv-key">{t('kv.keyLabel')}</Label>
          <Input
            id="kv-key"
            value={key}
            readOnly={!draft.isNew}
            onChange={(e) => {
              setKey(e.target.value)
            }}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="kv-ttl">{t('kv.ttlLabel')}</Label>
          <Input
            id="kv-ttl"
            inputMode="numeric"
            value={ttl}
            placeholder={t('kv.ttlPlaceholder')}
            onChange={(e) => {
              setTtl(e.target.value)
            }}
          />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="kv-value">{t('kv.valueLabel')}</Label>
        <Textarea
          id="kv-value"
          rows={6}
          className="font-mono text-xs"
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
          }}
        />
      </div>
      {put.error ? (
        <p role="alert" className="text-sm text-destructive">
          {put.error.message}
        </p>
      ) : null}
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="outline" onClick={onDone}>
          {t('kv.cancel')}
        </Button>
        <Button type="submit" disabled={put.isPending || key.trim() === ''}>
          {t('kv.save')}
        </Button>
      </div>
    </form>
  )
}

/** FunctionKVTab browses the function's key-value store; values stay truncated until a key is opened. */
export function FunctionKVTab({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('functions')
  const [prefix, setPrefix] = useState('')
  const list = useFunctionKV(name, prefix)
  const del = useDeleteKV(name)
  const rotate = useRotateKVToken(name)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [toDelete, setToDelete] = useState<KVKey | null>(null)

  async function edit(k: KVKey) {
    try {
      const full = await fetchKVValue(name, k.key)
      setDraft({ key: k.key, value: full.value, ttl: '', isNew: false })
    } catch (err) {
      toast.add({
        title: err instanceof Error ? err.message : t('kv.loadFailed'),
        type: 'error',
      })
    }
  }

  if (list.isError) {
    return (
      <p className="text-sm text-destructive">
        {t('kv.error', { message: list.error.message })}
      </p>
    )
  }
  const data = list.data
  if (!data)
    return <p className="text-sm text-muted-foreground">{t('kv.loading')}</p>

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          {t('kv.usage', {
            keys: data.count,
            maxKeys: data.limits.max_keys,
            bytes: data.bytes,
            maxBytes: data.limits.max_total_bytes,
          })}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={rotate.isPending}
            onClick={() => {
              rotate.mutate(undefined, {
                onSuccess: () =>
                  toast.add({ title: t('kv.tokenIssued'), type: 'success' }),
              })
            }}
          >
            {data.enabled ? t('kv.rotate') : t('kv.enable')}
          </Button>
          <Button
            type="button"
            disabled={!data.enabled}
            onClick={() => {
              setDraft(EMPTY_DRAFT)
            }}
          >
            {t('kv.add')}
          </Button>
        </div>
      </div>
      {rotate.error ? (
        <p role="alert" className="text-sm text-destructive">
          {rotate.error.message}
        </p>
      ) : null}
      {data.enabled ? null : (
        <p className="text-sm text-muted-foreground">{t('kv.disabled')}</p>
      )}
      {draft ? (
        <EditorForm
          key={draft.key + String(draft.isNew)}
          name={name}
          draft={draft}
          onDone={() => {
            setDraft(null)
          }}
        />
      ) : null}
      <div className="max-w-sm space-y-1.5">
        <Label htmlFor="kv-prefix">{t('kv.prefixLabel')}</Label>
        <Input
          id="kv-prefix"
          value={prefix}
          onChange={(e) => {
            setPrefix(e.target.value)
          }}
        />
      </div>
      {data.keys.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('kv.empty')}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('kv.columns.key')}</TableHead>
              <TableHead>{t('kv.columns.value')}</TableHead>
              <TableHead>{t('kv.columns.size')}</TableHead>
              <TableHead>{t('kv.columns.expires')}</TableHead>
              <TableHead className="text-right">
                {t('kv.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.keys.map((k) => (
              <TableRow key={k.key}>
                <TableCell className="font-mono text-xs">{k.key}</TableCell>
                <TableCell className="max-w-xs truncate font-mono text-xs text-muted-foreground">
                  {k.preview ?? t('kv.binary')}
                </TableCell>
                <TableCell>{t('kv.bytes', { count: k.size })}</TableCell>
                <TableCell>
                  {k.expires_at
                    ? new Date(k.expires_at).toLocaleString()
                    : t('kv.never')}
                </TableCell>
                <TableCell className="space-x-2 text-right">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      void edit(k)
                    }}
                  >
                    {t('kv.edit')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setToDelete(k)
                    }}
                  >
                    {t('kv.delete')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <ConfirmDialog
        open={toDelete !== null}
        onOpenChange={(open) => {
          if (!open) setToDelete(null)
        }}
        title={t('kv.deleteTitle', { key: toDelete?.key ?? '' })}
        description={t('kv.deleteBody')}
        confirmLabel={t('kv.delete')}
        tone="destructive"
        pending={del.isPending}
        error={del.error?.message ?? null}
        onConfirm={() => {
          if (!toDelete) return
          del.mutate(toDelete.key, {
            onSuccess: () => {
              setToDelete(null)
            },
          })
        }}
      />
    </div>
  )
}
