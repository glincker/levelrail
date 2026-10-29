import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useUnlinkProvider } from '../../queries/accountSelf'
import { passkeysQueryOptions } from '../../queries/passkeys'
import { myAccessQueryOptions } from '../../queries/myAccess'
import { useSession } from '../../queries/security'

export const ACCOUNT_PASSWORD_ANCHOR = 'account-password'
export const ACCOUNT_PASSKEYS_ANCHOR = 'passkeys'

function jumpTo(id: string, close: () => void) {
  close()
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' })
}

export function UnlinkProviderButton({
  provider,
  label,
}: {
  provider: string
  label: string
}) {
  const { t } = useTranslation('account')
  const { data: session } = useSession()
  const { data: access } = useQuery(myAccessQueryOptions())
  const { data: passkeys } = useQuery(passkeysQueryOptions())
  const unlink = useUnlinkProvider()
  const [open, setOpen] = useState(false)
  const [serverBlocked, setServerBlocked] = useState(false)

  const otherProviders = (access?.linked_providers ?? []).filter(
    (p) => p !== provider,
  ).length
  const lastWayIn =
    serverBlocked ||
    (!session.has_password &&
      otherProviders === 0 &&
      (passkeys?.length ?? 0) === 0)

  const close = () => setOpen(false)
  const confirm = () => {
    unlink.mutate(provider, {
      onSuccess: () => {
        close()
        toast.add({
          title: t('unlink.done', { provider: label }),
          type: 'success',
        })
      },
      onError: (e) => {
        if (e.code === 'last_sign_in_method') {
          setServerBlocked(true)
          return
        }
        toast.add({
          title: t('unlink.failed', { provider: label }),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        aria-label={t('unlink.actionLabel', { provider: label })}
        onClick={() => {
          setServerBlocked(false)
          setOpen(true)
        }}
      >
        {t('unlink.action')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        tone="destructive"
        title={t('unlink.title', { provider: label })}
        description={t('unlink.description', { provider: label })}
        confirmLabel={t('unlink.confirm')}
        pendingLabel={t('unlink.pending')}
        pending={unlink.isPending}
        onConfirm={() => {
          if (!lastWayIn) confirm()
        }}
      >
        {lastWayIn ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertTitle>{t('unlink.lastMethodTitle')}</AlertTitle>
            <AlertDescription className="space-y-3">
              <p>{t('unlink.lastMethodDescription', { provider: label })}</p>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  size="sm"
                  onClick={() => jumpTo(ACCOUNT_PASSWORD_ANCHOR, close)}
                >
                  {t('unlink.setPassword')}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => jumpTo(ACCOUNT_PASSKEYS_ANCHOR, close)}
                >
                  {t('unlink.addPasskey')}
                </Button>
              </div>
            </AlertDescription>
          </Alert>
        ) : null}
      </ConfirmDialog>
    </>
  )
}
