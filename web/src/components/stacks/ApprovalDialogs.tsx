import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { toast } from '@/components/ui/toast'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { ApiError } from '../../lib/apiError'
import { useApproveStack, useRejectStack } from '../../queries/stacks'
import type { StackRevisionSummary } from '../../types/stacks'

interface SecretRow {
  id: number
  name: string
  value: string
}

// Keeps the last non-null value so a closing dialog keeps its title while
// the exit animation runs.
function useLastValue<T>(value: T | null): T | null {
  const [last, setLast] = useState<T | null>(value)
  if (value !== null && value !== last) setLast(value)
  return value ?? last
}

export function ApproveDialog({
  name,
  revision,
  onClose,
}: Readonly<{
  name: string
  revision: StackRevisionSummary | null
  onClose: () => void
}>) {
  const { t } = useTranslation('stacks')
  const approve = useApproveStack(name)
  const username = useAuthUsername()
  const shown = useLastValue(revision)
  const isRequester = shown !== null && !!username && shown.actor === username
  const [rows, setRows] = useState<SecretRow[]>([])
  const [nextId, setNextId] = useState(1)
  const [error, setError] = useState<string | null>(null)

  // Secrets live only in this dialog's state and are dropped on every exit.
  function reset() {
    setRows([])
    setError(null)
    approve.reset()
  }

  function update(id: number, patch: Partial<SecretRow>) {
    setRows((cur) => cur.map((r) => (r.id === id ? { ...r, ...patch } : r)))
  }

  function confirm() {
    if (!revision) return
    const secrets: Record<string, string> = {}
    for (const r of rows) {
      const key = r.name.trim()
      if (key) secrets[key] = r.value
    }
    setError(null)
    approve.mutate(
      {
        revision: revision.number,
        secrets,
        expectedPlanHash: revision.plan_hash,
      },
      {
        onSuccess: (res) => {
          toast.add({
            title: t('approval.approved', { number: revision.number }),
            description: t('approval.approvedStatus', {
              status: t(`status.${res.revision.status}`),
            }),
            type: res.revision.status === 'applied' ? 'success' : 'warning',
          })
          reset()
          onClose()
        },
        onError: (e) => {
          setError(
            e instanceof ApiError && e.status === 409
              ? t('approval.stale')
              : e instanceof ApiError && e.status === 410
                ? t('approval.expired')
                : e.message,
          )
        },
      },
    )
  }

  return (
    <ConfirmDialog
      open={revision !== null}
      onOpenChange={(open) => {
        if (!open) {
          reset()
          onClose()
        }
      }}
      title={t('approval.approveTitle', { number: shown?.number ?? 0 })}
      description={t(
        isRequester
          ? 'approval.approveDescriptionOwn'
          : 'approval.approveDescription',
        { name },
      )}
      consequences={[t('approval.secretsNote')]}
      confirmDisabled={isRequester}
      confirmLabel={t('approval.approve')}
      pendingLabel={t('approval.approving')}
      pending={approve.isPending}
      error={error}
      onConfirm={confirm}
    >
      <div className="space-y-1.5">
        {rows.map((r) => (
          <div key={r.id} className="flex items-center gap-1.5">
            <Input
              value={r.name}
              placeholder={t('approval.secretName')}
              aria-label={t('approval.secretName')}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => {
                update(r.id, { name: e.target.value })
              }}
            />
            <SecretInput
              value={r.value}
              placeholder={t('approval.secretValue')}
              aria-label={t('approval.secretValue')}
              onChange={(e) => {
                update(r.id, { value: e.target.value })
              }}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('approval.removeSecret')}
              onClick={() => {
                setRows((cur) => cur.filter((x) => x.id !== r.id))
              }}
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
            </Button>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            setRows((cur) => [...cur, { id: nextId, name: '', value: '' }])
            setNextId((n) => n + 1)
          }}
        >
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('approval.addSecret')}
        </Button>
      </div>
    </ConfirmDialog>
  )
}

export function RejectDialog({
  name,
  revision,
  onClose,
}: Readonly<{
  name: string
  revision: number | null
  onClose: () => void
}>) {
  const { t } = useTranslation('stacks')
  const reject = useRejectStack(name)
  const shown = useLastValue(revision)
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)

  function close() {
    setReason('')
    setError(null)
    reject.reset()
    onClose()
  }

  function confirm() {
    if (revision === null) return
    setError(null)
    reject.mutate(
      { revision, reason: reason.trim() },
      {
        onSuccess: () => {
          toast.add({
            title: t('approval.rejected', { number: revision }),
            type: 'success',
          })
          close()
        },
        onError: (e) => {
          setError(e.message)
        },
      },
    )
  }

  return (
    <ConfirmDialog
      open={revision !== null}
      onOpenChange={(open) => {
        if (!open) close()
      }}
      title={t('approval.rejectTitle', { number: shown ?? 0 })}
      description={t('approval.rejectDescription', { name })}
      tone="destructive"
      confirmLabel={t('approval.reject')}
      pendingLabel={t('approval.rejecting')}
      pending={reject.isPending}
      error={error}
      onConfirm={confirm}
    >
      <Input
        value={reason}
        placeholder={t('approval.reason')}
        aria-label={t('approval.reason')}
        onChange={(e) => {
          setReason(e.target.value)
        }}
      />
    </ConfirmDialog>
  )
}
