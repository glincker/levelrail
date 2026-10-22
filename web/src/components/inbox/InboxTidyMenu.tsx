import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from '@/components/ui/toast'
import {
  clearInbox,
  useClearInbox,
  type InboxClearAction,
  type InboxClearScope,
} from '../../queries/inbox'

type TidyId =
  'archiveRead' | 'archiveResolved' | 'clearResolved' | 'deleteArchived'

interface TidyAction {
  id: TidyId
  action: InboxClearAction
  scope: InboxClearScope
}

const ACTIONS: TidyAction[] = [
  { id: 'archiveRead', action: 'archive', scope: 'read' },
  { id: 'archiveResolved', action: 'archive', scope: 'resolved' },
  { id: 'clearResolved', action: 'delete', scope: 'resolved' },
  { id: 'deleteArchived', action: 'delete', scope: 'archived' },
]

interface Pending {
  item: TidyAction
  count: number
}

export default function InboxTidyMenu() {
  const { t } = useTranslation('inbox')
  const clear = useClearInbox()
  const [pending, setPending] = useState<Pending | null>(null)

  const run = (item: TidyAction, confirmCount?: number) => {
    clear.mutate(
      {
        action: item.action,
        scope: item.scope,
        confirm_count: confirmCount,
      },
      {
        onSuccess: (res) => {
          setPending(null)
          toast.add({
            title: t(`tidy.done.${item.id}`, { count: res.count }),
            type: 'success',
          })
        },
        onError: () => {
          toast.add({ title: t('tidy.error'), type: 'error' })
        },
      },
    )
  }

  const choose = async (item: TidyAction) => {
    try {
      const preview = await clearInbox({
        action: item.action,
        scope: item.scope,
        dry_run: true,
      })
      if (preview.count === 0) {
        toast.add({ title: t('tidy.nothing'), type: 'info' })
      } else if (item.action === 'delete') {
        setPending({ item, count: preview.count })
      } else {
        run(item)
      }
    } catch {
      toast.add({ title: t('tidy.error'), type: 'error' })
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button type="button" size="sm" variant="outline">
              <BroomIcon />
              {t('tidy.menu')}
            </Button>
          }
        />
        <DropdownMenuContent>
          {ACTIONS.map((a) => (
            <DropdownMenuItem
              key={a.id}
              onClick={() => {
                void choose(a)
              }}
            >
              {t(`tidy.${a.id}`)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null)
        }}
        title={pending ? t(`tidy.${pending.item.id}`) : ''}
        description={
          pending ? t('tidy.confirm', { count: pending.count }) : undefined
        }
        confirmLabel={t('tidy.confirmAction')}
        tone="destructive"
        pending={clear.isPending}
        onConfirm={() => {
          if (pending) run(pending.item, pending.count)
        }}
      />
    </>
  )
}
