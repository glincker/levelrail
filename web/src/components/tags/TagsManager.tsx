import { Fragment, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  PencilGlassIcon,
  PlusGlassIcon,
  TagGlassIcon,
  TrashGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { useAbilities } from '@/hooks/useAbilities'
import { formatDate } from '@/lib/format'
import { useQuery } from '@tanstack/react-query'
import { tagsQueryOptions, type Tag } from '@/queries/tags'
import {
  useCreateTag,
  useDeleteTag,
  useRenameTag,
  useTagApps,
} from '@/queries/tagsAdmin'

function TagApps({ tag }: { tag: Tag }) {
  const { t } = useTranslation('tags')
  const apps = useTagApps(tag.id, true)
  if (apps.isPending) {
    return <p className="text-sm text-muted-foreground">{t('apps.loading')}</p>
  }
  if (apps.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {t('apps.error')}
      </p>
    )
  }
  if (apps.data.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('apps.empty')}</p>
  }
  return (
    <ul
      aria-label={t('apps.list', { name: tag.name })}
      className="flex flex-wrap gap-2"
    >
      {apps.data.map((app) => (
        <li key={app.name}>
          <Link
            to="/apps/$name"
            params={{ name: app.name }}
            className="rounded-md border border-border px-2 py-0.5 text-sm hover:bg-muted"
          >
            {app.name}
          </Link>
        </li>
      ))}
    </ul>
  )
}

function RenameForm({ tag, onDone }: { tag: Tag; onDone: () => void }) {
  const { t } = useTranslation('tags')
  const rename = useRenameTag()
  const [draft, setDraft] = useState(tag.name)

  function submit(e: FormEvent) {
    e.preventDefault()
    const name = draft.trim()
    if (name === '' || name === tag.name) {
      onDone()
      return
    }
    rename.mutate({ id: tag.id, name }, { onSuccess: onDone })
  }

  return (
    <form onSubmit={submit} className="flex flex-wrap items-center gap-2">
      <Input
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        aria-label={t('rename.label', { name: tag.name })}
        aria-invalid={rename.isError}
        className="max-w-56"
      />
      <Button
        type="submit"
        size="icon-sm"
        variant="outline"
        disabled={rename.isPending}
        aria-label={t('rename.save')}
      >
        <CheckGlassIcon size="1.2em" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        onClick={onDone}
        aria-label={t('rename.cancel')}
      >
        <XGlassIcon size="1.2em" />
      </Button>
      {rename.isError && (
        <span role="alert" className="text-sm text-destructive">
          {rename.error.message}
        </span>
      )}
    </form>
  )
}

function CreateForm() {
  const { t } = useTranslation('tags')
  const create = useCreateTag()
  const [name, setName] = useState('')

  function submit(e: FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (trimmed === '') return
    create.mutate(trimmed, { onSuccess: () => setName('') })
  }

  return (
    <form onSubmit={submit} className="flex flex-wrap items-center gap-2">
      <Input
        value={name}
        onChange={(e) => setName(e.target.value)}
        aria-label={t('create.label')}
        aria-invalid={create.isError}
        placeholder={t('create.placeholder')}
        className="max-w-64"
      />
      <Button
        type="submit"
        size="sm"
        disabled={create.isPending || name.trim() === ''}
      >
        <PlusGlassIcon size="1.4em" />
        {create.isPending ? t('create.pending') : t('create.submit')}
      </Button>
      {create.isError && (
        <span role="alert" className="text-sm text-destructive">
          {create.error.message}
        </span>
      )}
    </form>
  )
}

function TagRow({ tag, canWrite }: { tag: Tag; canWrite: boolean }) {
  const { t } = useTranslation('tags')
  const del = useDeleteTag()
  const [open, setOpen] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const Chevron = open ? ChevronDownGlassIcon : ChevronRightGlassIcon

  return (
    <Fragment>
      <TableRow>
        <TableCell className="font-medium">
          {renaming ? (
            <RenameForm tag={tag} onDone={() => setRenaming(false)} />
          ) : (
            <button
              type="button"
              aria-expanded={open}
              aria-label={t(open ? 'row.hideApps' : 'row.showApps', {
                name: tag.name,
              })}
              onClick={() => setOpen((v) => !v)}
              className="flex items-center gap-1.5 rounded-md text-left outline-none focus-visible:ring-1 focus-visible:ring-foreground/30"
            >
              <Chevron size="1.2em" />
              {tag.name}
            </button>
          )}
        </TableCell>
        <TableCell className="tabular-nums">{tag.app_count ?? 0}</TableCell>
        <TableCell className="tabular-nums">
          {tag.resource_count ?? 0}
        </TableCell>
        <TableCell className="text-muted-foreground">
          {formatDate(tag.created_at, '')}
        </TableCell>
        {canWrite && (
          <TableCell>
            <div className="flex justify-end gap-1">
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={t('rename.action', { name: tag.name })}
                onClick={() => setRenaming(true)}
              >
                <PencilGlassIcon size="1.2em" />
              </Button>
              <ConfirmAction
                trigger={(openDialog) => (
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t('delete.action', { name: tag.name })}
                    onClick={openDialog}
                  >
                    <TrashGlassIcon size="1.2em" />
                  </Button>
                )}
                title={t('delete.title', { name: tag.name })}
                description={t('delete.description')}
                consequences={[t('delete.consequence')]}
                confirmLabel={t('delete.confirm')}
                pendingLabel={t('delete.pending')}
                pending={del.isPending}
                error={del.isError ? del.error.message : null}
                onConfirm={(close) => del.mutate(tag.id, { onSuccess: close })}
                onOpenChange={() => del.reset()}
              />
            </div>
          </TableCell>
        )}
      </TableRow>
      {open && (
        <TableRow>
          <TableCell colSpan={canWrite ? 5 : 4} className="bg-muted/30">
            <TagApps tag={tag} />
          </TableCell>
        </TableRow>
      )}
    </Fragment>
  )
}

export function TagsManager() {
  const { t } = useTranslation('tags')
  const { can } = useAbilities()
  const canWrite = can('write')
  const tags = useQuery({ ...tagsQueryOptions(), retry: false })

  return (
    <PageTemplate
      title={t('page.title')}
      description={t('page.description')}
      icon={<TagGlassIcon size="1.4em" />}
    >
      <div className="flex flex-col gap-4">
        {canWrite && <CreateForm />}
        {tags.isError && (
          <p role="alert" className="text-sm text-destructive">
            {t('error')}
          </p>
        )}
        {tags.isSuccess && tags.data.length === 0 && (
          <EmptyState
            icon={<TagGlassIcon className="size-6" />}
            title={t('empty.title')}
            description={t('empty.description')}
          />
        )}
        {tags.isSuccess && tags.data.length > 0 && (
          <Table aria-label={t('table.label')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('table.name')}</TableHead>
                <TableHead>{t('table.apps')}</TableHead>
                <TableHead>{t('table.resources')}</TableHead>
                <TableHead>{t('table.created')}</TableHead>
                {canWrite && (
                  <TableHead className="text-right">
                    <span className="sr-only">{t('table.actions')}</span>
                  </TableHead>
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {tags.data.map((tag) => (
                <TagRow key={tag.id} tag={tag} canWrite={canWrite} />
              ))}
            </TableBody>
          </Table>
        )}
      </div>
    </PageTemplate>
  )
}
