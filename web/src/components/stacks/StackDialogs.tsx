import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useDeleteStack, useRollbackStack } from '../../queries/stacks'
import type { StackDeleteMode } from '../../types/stacks'

export function RollbackDialog({
  name,
  revision,
  onClose,
}: Readonly<{ name: string; revision: number | null; onClose: () => void }>) {
  const { t } = useTranslation('stacks')
  const rollback = useRollbackStack(name)
  const [error, setError] = useState<string | null>(null)

  function confirm() {
    if (revision === null) return
    setError(null)
    rollback.mutate(revision, {
      onSuccess: (res) => {
        toast.add({
          title: t('rollback.done', { number: revision }),
          description: t('rollback.doneStatus', {
            number: res.revision.number,
            status: t(`status.${res.revision.status}`),
          }),
          type: res.revision.status === 'applied' ? 'success' : 'warning',
        })
        onClose()
      },
      onError: (e) => {
        setError(e.message)
      },
    })
  }

  return (
    <ConfirmDialog
      open={revision !== null}
      onOpenChange={(open) => {
        if (!open) {
          setError(null)
          onClose()
        }
      }}
      title={t('rollback.title', { number: revision ?? 0 })}
      description={t('rollback.description', {
        number: revision ?? 0,
        name,
      })}
      consequences={[t('rollback.secretsNote'), t('rollback.dataNote')]}
      confirmLabel={t('rollback.confirm')}
      pendingLabel={t('rollback.pending')}
      pending={rollback.isPending}
      error={error}
      onConfirm={confirm}
    />
  )
}

// Destroy is root-only on the server; non-admins only see the forget choice.
export function DeleteStackDialog({
  name,
  open,
  isRoot,
  onClose,
  onDeleted,
}: Readonly<{
  name: string
  open: boolean
  isRoot: boolean
  onClose: () => void
  onDeleted: () => void
}>) {
  const { t } = useTranslation('stacks')
  const remove = useDeleteStack(name)
  const [mode, setMode] = useState<StackDeleteMode>('forget')
  const [error, setError] = useState<string | null>(null)
  const modes: StackDeleteMode[] = isRoot ? ['forget', 'destroy'] : ['forget']
  const active: StackDeleteMode = isRoot ? mode : 'forget'

  function confirm() {
    setError(null)
    remove.mutate(active, {
      onSuccess: () => {
        toast.add({ title: t('delete.done', { name }), type: 'success' })
        onDeleted()
      },
      onError: (e) => {
        setError(e.message)
      },
    })
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setError(null)
          onClose()
        }
      }}
      title={t('delete.title', { name })}
      description={t('delete.description')}
      tone="destructive"
      confirmLabel={t('delete.confirm')}
      pendingLabel={t('delete.pending')}
      pending={remove.isPending}
      requireTyped={active === 'destroy' ? name : undefined}
      error={error}
      onConfirm={confirm}
    >
      <fieldset className="space-y-2">
        <legend className="sr-only">{t('delete.description')}</legend>
        {modes.map((m) => (
          <label
            key={m}
            className="flex cursor-pointer items-start gap-2 rounded-lg border border-border p-2 has-checked:border-ring"
          >
            <input
              type="radio"
              name="stack-delete-mode"
              className="mt-1"
              checked={active === m}
              onChange={() => {
                setMode(m)
              }}
            />
            <span className="space-y-0.5">
              <span className="block text-sm font-medium">
                {t(`delete.${m}`)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t(`delete.${m}Hint`)}
              </span>
            </span>
          </label>
        ))}
      </fieldset>
    </ConfirmDialog>
  )
}
