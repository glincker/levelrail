import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon, PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { normalizeRegistryHost } from '../../lib/registryHost'
import {
  useAddPullSecret,
  useDeletePullSecret,
  usePullSecrets,
} from '../../queries/clusters'

const NAME_RE = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/

export function PullSecretsPanel({ cluster }: Readonly<{ cluster: string }>) {
  const { t } = useTranslation('clusters')
  const list = usePullSecrets(cluster)
  const add = useAddPullSecret(cluster)
  const del = useDeletePullSecret(cluster)
  const [name, setName] = useState('')
  const [registry, setRegistry] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const canSubmit =
    NAME_RE.test(name) &&
    normalizeRegistryHost(registry) !== '' &&
    username.trim() !== '' &&
    password !== '' &&
    !add.isPending

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    add.mutate(
      { name, registry: registry.trim(), username: username.trim(), password },
      {
        onSuccess: () => {
          toast.add({
            title: t('pullSecrets.added', { name }),
            type: 'success',
          })
          // Write only: nothing typed is kept once the server has it.
          setName('')
          setRegistry('')
          setUsername('')
          setPassword('')
        },
      },
    )
  }

  return (
    <section
      id="pull-secrets"
      aria-label={t('pullSecrets.title')}
      className="flex scroll-mt-4 flex-col gap-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-baseline gap-x-3">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold">
          <KeyGlassIcon aria-hidden="true" className="size-4" />
          {t('pullSecrets.title')}
        </h2>
        <p className="text-xs text-muted-foreground">{t('pullSecrets.hint')}</p>
      </div>
      {list.isPending ? (
        <Skeleton className="h-5 w-48" />
      ) : list.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {list.error.message}
        </p>
      ) : list.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('pullSecrets.empty')}
        </p>
      ) : (
        <ul className="flex flex-col divide-y divide-border">
          {list.data.map((s) => (
            <li
              key={s.name}
              className="flex flex-wrap items-center gap-x-3 py-1 text-sm"
            >
              <span className="min-w-0 truncate font-medium">{s.name}</span>
              <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {s.registry}
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                aria-label={t('pullSecrets.deleteNamed', { name: s.name })}
                onClick={() => {
                  setDeleting(s.name)
                }}
              >
                <TrashGlassIcon aria-hidden="true" />
                {t('pullSecrets.delete')}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        onSubmit={submit}
        aria-label={t('pullSecrets.addTitle')}
        className="grid items-end gap-2 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_1fr_1fr_auto]"
      >
        <div className="flex flex-col gap-1">
          <Label htmlFor="ps-name">{t('pullSecrets.name')}</Label>
          <Input
            id="ps-name"
            value={name}
            autoComplete="off"
            placeholder={t('pullSecrets.namePlaceholder')}
            aria-invalid={name !== '' && !NAME_RE.test(name)}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="ps-registry">{t('pullSecrets.registry')}</Label>
          <Input
            id="ps-registry"
            value={registry}
            autoComplete="off"
            placeholder={t('pullSecrets.registryPlaceholder')}
            onChange={(e) => {
              setRegistry(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="ps-username">{t('pullSecrets.username')}</Label>
          <Input
            id="ps-username"
            value={username}
            autoComplete="off"
            onChange={(e) => {
              setUsername(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="ps-password">{t('pullSecrets.password')}</Label>
          <Input
            id="ps-password"
            type="password"
            value={password}
            autoComplete="new-password"
            onChange={(e) => {
              setPassword(e.target.value)
            }}
          />
        </div>
        <Button type="submit" size="sm" disabled={!canSubmit}>
          <PlusGlassIcon size="1.2em" aria-hidden="true" />
          {add.isPending ? t('pullSecrets.adding') : t('pullSecrets.add')}
        </Button>
      </form>
      {add.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {add.error.message}
        </p>
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => {
          if (!o) setDeleting(null)
        }}
        tone="destructive"
        title={t('pullSecrets.deleteTitle', { name: deleting ?? '' })}
        description={t('pullSecrets.deleteDescription')}
        requireTyped={deleting ?? ''}
        confirmLabel={t('pullSecrets.deleteConfirm')}
        pendingLabel={t('pullSecrets.deletePending')}
        pending={del.isPending}
        error={del.error?.message ?? null}
        focusCancel
        onConfirm={() => {
          if (deleting === null) return
          const target = deleting
          del.mutate(target, {
            onSuccess: () => {
              setDeleting(null)
              toast.add({
                title: t('pullSecrets.deleted', { name: target }),
                type: 'success',
              })
            },
          })
        }}
      />
    </section>
  )
}
