import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  PencilGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
  UsersGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { toast } from '@/components/ui/toast'
import { AudienceDialog } from '../../components/mail/AudienceDialog'
import { MailPageShell } from '../../components/mail/MailPageShell'
import { SuppressionCard } from '../../components/mail/SuppressionCard'
import {
  useDeleteMailGroup,
  useMailGroups,
  type MailGroup,
} from '../../queries/mail'

export const Route = createFileRoute('/settings/email-audiences')({
  component: EmailAudiencesPage,
})

function EmailAudiencesPage() {
  const { t } = useTranslation('mail')
  const { data: groups = [], isLoading, error, refetch } = useMailGroups()
  const del = useDeleteMailGroup()
  const [dialog, setDialog] = useState<{ group: MailGroup | null } | null>(null)
  const [deleting, setDeleting] = useState<MailGroup | null>(null)

  const createButton = (
    <Button
      type="button"
      size="sm"
      writeAction
      onClick={() => setDialog({ group: null })}
    >
      <PlusGlassIcon size="1.2em" data-icon="inline-start" />
      {t('audiences.create')}
    </Button>
  )

  return (
    <MailPageShell section="audiences" actions={createButton}>
      <div className="space-y-6">
        {isLoading ? <TableSkeleton columnCount={3} rowCount={4} /> : null}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
              <span>{error.message || t('common.loadError')}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void refetch()}
              >
                {t('common.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {!isLoading && !error && groups.length === 0 ? (
          <EmptyState
            icon={<UsersGlassIcon className="size-5.5" />}
            title={t('audiences.empty.title')}
            description={t('audiences.empty.description')}
            action={createButton}
          />
        ) : null}
        {groups.length > 0 ? (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {groups.map((g) => (
              <li
                key={g.id}
                className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium">{g.name}</p>
                  {g.description ? (
                    <p className="text-sm text-muted-foreground">
                      {g.description}
                    </p>
                  ) : null}
                  <p className="text-xs text-muted-foreground">
                    {t('common.people', { count: g.recipients })}
                    {g.suppressed > 0
                      ? `, ${t('audiences.columns.suppressed')}: ${g.suppressed}`
                      : ''}
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setDialog({ group: g })}
                  >
                    <PencilGlassIcon size="1.2em" data-icon="inline-start" />
                    {t('audiences.edit')}
                  </Button>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`${t('common.delete')} ${g.name}`}
                    writeAction
                    onClick={() => setDeleting(g)}
                  >
                    <TrashGlassIcon size="1.2em" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        ) : null}
        <SuppressionCard />
      </div>
      {dialog ? (
        <AudienceDialog
          key={dialog.group?.id ?? 'new'}
          group={dialog.group}
          open
          onOpenChange={(open) => {
            if (!open) setDialog(null)
          }}
        />
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title={t('audiences.deleteTitle')}
        description={t('audiences.deleteDescription')}
        tone="destructive"
        confirmLabel={t('common.delete')}
        pending={del.isPending}
        onConfirm={() => {
          if (!deleting) return
          del.mutate(deleting.id, {
            onSuccess: () => {
              toast.add({ title: t('audiences.deleted'), type: 'success' })
              setDeleting(null)
            },
            onError: (e) => toast.add({ title: e.message, type: 'error' }),
          })
        }}
      />
    </MailPageShell>
  )
}
