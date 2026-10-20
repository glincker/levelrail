import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { EnvelopeIcon, PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { BroadcastComposer } from '../../components/mail/BroadcastComposer'
import { BroadcastHistory } from '../../components/mail/BroadcastHistory'
import { MailPageShell } from '../../components/mail/MailPageShell'
import { useMailBroadcasts } from '../../queries/mail'

export const Route = createFileRoute('/settings/email-messages')({
  component: EmailMessagesPage,
})

function EmailMessagesPage() {
  const { t } = useTranslation('mail')
  const {
    data: broadcasts = [],
    isLoading,
    error,
    refetch,
  } = useMailBroadcasts()
  const [composing, setComposing] = useState(false)

  const composeButton = (
    <Button
      type="button"
      size="sm"
      writeAction
      onClick={() => setComposing(true)}
    >
      <PlusIcon data-icon="inline-start" />
      {t('messages.compose')}
    </Button>
  )

  return (
    <MailPageShell section="messages" actions={composeButton}>
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
      {!isLoading && !error && broadcasts.length === 0 ? (
        <EmptyState
          icon={<EnvelopeIcon className="size-5" />}
          title={t('messages.history.empty.title')}
          description={t('messages.history.empty.description')}
          action={composeButton}
        />
      ) : null}
      {broadcasts.length > 0 ? (
        <BroadcastHistory broadcasts={broadcasts} />
      ) : null}
      {composing ? (
        <BroadcastComposer open onOpenChange={setComposing} />
      ) : null}
    </MailPageShell>
  )
}
