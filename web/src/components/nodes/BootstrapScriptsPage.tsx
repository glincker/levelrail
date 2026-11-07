import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  PencilGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { useIsRoot } from '@/hooks/useIsRoot'
import { formatBytes, formatDate } from '@/lib/format'
import {
  bootstrapLimitsQueryOptions,
  bootstrapScriptsQueryOptions,
  fetchBootstrapScript,
  useDeleteBootstrapScript,
  useSaveBootstrapScript,
  type BootstrapScript,
} from '@/queries/bootstrapScripts'

interface Draft {
  id?: string
  name: string
  body: string
}

const encoder = new TextEncoder()

function Editor({
  draft,
  maxBytes,
  onDone,
}: {
  draft: Draft
  maxBytes: number | undefined
  onDone: () => void
}) {
  const { t } = useTranslation('nodes', { keyPrefix: 'bootstrapScripts' })
  const save = useSaveBootstrapScript()
  const [name, setName] = useState(draft.name)
  const [body, setBody] = useState(draft.body)
  const size = encoder.encode(body).length
  const over = maxBytes !== undefined && size > maxBytes
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate({ id: draft.id, name, body }, { onSuccess: onDone })
      }}
    >
      <div className="flex flex-wrap items-center gap-2">
        <label htmlFor="bs-name" className="text-sm">
          {t('editor.name')}
        </label>
        <Input
          id="bs-name"
          value={name}
          maxLength={64}
          onChange={(e) => setName(e.target.value)}
          className="max-w-xs"
        />
      </div>
      <div className="space-y-1">
        <label htmlFor="bs-body" className="text-sm">
          {t('editor.body')}
        </label>
        <Textarea
          id="bs-body"
          value={body}
          rows={12}
          spellCheck={false}
          onChange={(e) => setBody(e.target.value)}
          className="font-mono text-xs"
        />
        <p
          className={
            over ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'
          }
        >
          {maxBytes === undefined
            ? formatBytes(size)
            : t('editor.size', {
                used: formatBytes(size),
                max: formatBytes(maxBytes),
              })}
        </p>
      </div>
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <div className="flex gap-2">
        <Button
          type="submit"
          disabled={!name.trim() || !body || over || save.isPending}
        >
          {save.isPending ? t('editor.saving') : t('editor.save')}
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          {t('editor.cancel')}
        </Button>
      </div>
    </form>
  )
}

export function BootstrapScriptsPage() {
  const { t } = useTranslation('nodes', { keyPrefix: 'bootstrapScripts' })
  const isRoot = useIsRoot()
  const list = useQuery(bootstrapScriptsQueryOptions())
  const limits = useQuery({ ...bootstrapLimitsQueryOptions(), enabled: isRoot })
  const del = useDeleteBootstrapScript()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [loadError, setLoadError] = useState(false)

  async function edit(s: BootstrapScript) {
    setLoadError(false)
    try {
      const full = await fetchBootstrapScript(s.id)
      setDraft({ id: s.id, name: full.name, body: full.body ?? '' })
    } catch {
      setLoadError(true)
    }
  }

  return (
    <PageTemplate
      title={t('page.title')}
      description={t('page.description')}
      actions={
        isRoot && !draft ? (
          <Button onClick={() => setDraft({ name: '', body: '' })}>
            <PlusGlassIcon size="1.2em" />
            {t('new')}
          </Button>
        ) : null
      }
    >
      <p className="mb-3 text-sm text-muted-foreground">{t('runsAsRoot')}</p>
      {!isRoot ? (
        <p className="mb-3 text-sm text-muted-foreground">{t('rootOnly')}</p>
      ) : null}
      {loadError ? (
        <p role="alert" className="mb-3 text-sm text-destructive">
          {t('loadScriptError')}
        </p>
      ) : null}
      {draft ? (
        <Editor
          key={draft.id ?? 'new'}
          draft={draft}
          maxBytes={limits.data?.max_bytes}
          onDone={() => setDraft(null)}
        />
      ) : list.isPending ? (
        <p className="text-sm text-muted-foreground">{t('loading')}</p>
      ) : list.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('error')}
        </p>
      ) : list.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('empty')}</p>
      ) : (
        <Table aria-label={t('table.label')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('table.name')}</TableHead>
              <TableHead>{t('table.size')}</TableHead>
              <TableHead>{t('table.updated')}</TableHead>
              <TableHead>
                <span className="sr-only">{t('table.actions')}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.data.map((s) => (
              <TableRow key={s.id}>
                <TableCell className="font-medium">{s.name}</TableCell>
                <TableCell>{formatBytes(s.size_bytes)}</TableCell>
                <TableCell>{formatDate(s.updated_at, '-')}</TableCell>
                <TableCell className="text-right">
                  {isRoot ? (
                    <div className="flex justify-end gap-1">
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={t('edit', { name: s.name })}
                        onClick={() => void edit(s)}
                      >
                        <PencilGlassIcon size="1.2em" />
                      </Button>
                      <ConfirmAction
                        trigger={(openDialog) => (
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            aria-label={t('delete.action', { name: s.name })}
                            onClick={openDialog}
                          >
                            <TrashGlassIcon size="1.2em" />
                          </Button>
                        )}
                        title={t('delete.title', { name: s.name })}
                        consequences={[t('delete.consequence')]}
                        confirmLabel={t('delete.confirm')}
                        pending={del.isPending}
                        error={del.isError ? del.error.message : null}
                        onConfirm={(close) =>
                          del.mutate(s.id, { onSuccess: close })
                        }
                      />
                    </div>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </PageTemplate>
  )
}
