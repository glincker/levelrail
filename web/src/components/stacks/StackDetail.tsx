import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  LockOpenGlassIcon,
  PinGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { SkeletonLine } from '@/components/kit'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { formatAbsolute } from '../../lib/dateFormat'
import {
  stackHistoryQueryOptions,
  stackQueryOptions,
  usePinStack,
  useUnlockStack,
} from '../../queries/stacks'
import type { StackDiffAgainst, StackRevisionSummary } from '../../types/stacks'
import { ApproveDialog, RejectDialog } from './ApprovalDialogs'
import { RevisionHistory } from './RevisionHistory'
import { RevisionView } from './RevisionView'
import { DeleteStackDialog, RollbackDialog } from './StackDialogs'
import { RevisionStatusBadge, StackFlags } from './StackIndicators'
import { DriftSummary } from './StackDrift'
import { StackSettingsCard } from './StackSettingsCard'

export interface StackSelection {
  rev?: number
  diff: boolean
  against: StackDiffAgainst
}

export function StackDetail({
  name,
  isRoot,
  selection,
  onSelect,
  onBack,
}: Readonly<{
  name: string
  isRoot: boolean
  selection: StackSelection
  onSelect: (next: Partial<StackSelection>) => void
  onBack: () => void
}>) {
  const { t } = useTranslation('stacks')
  const stackQuery = useQuery(stackQueryOptions(name))
  const historyQuery = useQuery(stackHistoryQueryOptions(name))
  const pin = usePinStack(name)
  const unlock = useUnlockStack(name)
  const [rollbackRev, setRollbackRev] = useState<number | null>(null)
  const currentUser = useAuthUsername()
  const [approving, setApproving] = useState<StackRevisionSummary | null>(null)
  const [rejecting, setRejecting] = useState<number | null>(null)
  const [deleting, setDeleting] = useState(false)

  const stack = stackQuery.data
  if (stackQuery.isError) {
    return <p className="text-sm text-destructive">{t('detail.loadFailed')}</p>
  }
  if (!stack) return <SkeletonLine />

  const revisions = historyQuery.data?.revisions ?? []
  const fail = (e: Error) => {
    toast.add({
      title: t('actions.failed'),
      description: e.message,
      type: 'error',
    })
  }

  function setPin(n: number | null) {
    pin.mutate(n, {
      onSuccess: () => {
        toast.add({
          title:
            n === null
              ? t('actions.unpinned')
              : t('actions.pinned', { revision: n }),
          type: 'success',
        })
      },
      onError: fail,
    })
  }

  return (
    <div className="space-y-3">
      <header className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeftGlassIcon size="1.2em" aria-hidden="true" />
          {t('detail.back')}
        </Button>
        <h2 className="text-base font-semibold">{stack.name}</h2>
        <RevisionStatusBadge status={stack.latest_status} />
        <StackFlags stack={stack} />
        <span className="ml-auto flex items-center gap-1">
          {stack.pinned_revision ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={pin.isPending}
              onClick={() => {
                setPin(null)
              }}
            >
              <PinGlassIcon size="1.2em" aria-hidden="true" />
              {t('actions.unpin')}
            </Button>
          ) : null}
          {isRoot && stack.lock ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={unlock.isPending}
              onClick={() => {
                unlock.mutate(undefined, {
                  onSuccess: () => {
                    toast.add({ title: t('actions.unlocked'), type: 'success' })
                  },
                  onError: fail,
                })
              }}
            >
              <LockOpenGlassIcon size="1.2em" aria-hidden="true" />
              {t('actions.unlock')}
            </Button>
          ) : null}
          <Button
            type="button"
            variant="destructive"
            size="sm"
            onClick={() => {
              setDeleting(true)
            }}
          >
            <TrashGlassIcon size="1.2em" aria-hidden="true" />
            {t('actions.delete')}
          </Button>
        </span>
      </header>
      <p className="text-xs text-muted-foreground">
        {t('detail.inProject', { project: stack.project })}
        {' / '}
        {t('detail.createdBy', { name: stack.created_by })}
        {stack.resources ? (
          <>
            {' / '}
            {t('detail.resources', { count: stack.resources.length })}
          </>
        ) : null}
      </p>
      {stack.lock ? (
        <Banner
          variant="warning"
          title={t('detail.lockedBanner', {
            holder: stack.lock.holder,
            revision: stack.lock.revision,
            until: formatAbsolute(stack.lock.expires_at),
          })}
        />
      ) : null}
      {stack.pinned_revision ? (
        <Banner
          variant="info"
          title={t('detail.pinnedBanner', { revision: stack.pinned_revision })}
        />
      ) : null}
      <DriftSummary drift={stack.drift} />
      <h3 className="text-sm font-semibold">{t('history.title')}</h3>
      {revisions.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('history.empty')}</p>
      ) : (
        <RevisionHistory
          stack={stack}
          isRoot={isRoot}
          currentUser={currentUser}
          revisions={revisions}
          selected={selection.rev}
          onView={(n) => {
            onSelect({ rev: n, diff: false })
          }}
          onDiff={(n) => {
            onSelect({ rev: n, diff: true })
          }}
          onRollback={setRollbackRev}
          onPin={setPin}
          onApprove={setApproving}
          onReject={setRejecting}
        />
      )}
      {selection.rev !== undefined ? (
        <RevisionView
          name={name}
          revision={selection.rev}
          against={selection.against}
          otherRevisions={revisions
            .map((r) => r.number)
            .filter((n) => n !== selection.rev)
            .slice(0, 5)}
          showDiff={selection.diff}
          onAgainst={(against) => {
            onSelect({ against })
          }}
          onClose={() => {
            onSelect({ rev: undefined, diff: false })
          }}
        />
      ) : null}
      <StackSettingsCard stack={stack} isRoot={isRoot} />
      <ApproveDialog
        name={name}
        revision={approving}
        onClose={() => {
          setApproving(null)
        }}
      />
      <RejectDialog
        name={name}
        revision={rejecting}
        onClose={() => {
          setRejecting(null)
        }}
      />
      <RollbackDialog
        name={name}
        revision={rollbackRev}
        onClose={() => {
          setRollbackRev(null)
        }}
      />
      <DeleteStackDialog
        name={name}
        open={deleting}
        isRoot={isRoot}
        onClose={() => {
          setDeleting(false)
        }}
        onDeleted={() => {
          setDeleting(false)
          onBack()
        }}
      />
    </div>
  )
}
