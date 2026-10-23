import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useRevokeToken } from '../queries/tokens'
import type { TokenResource } from '../types/token'

export function RevokeTokenDialog({ token }: { token: TokenResource }) {
  const { t } = useTranslation('common')
  const revokeToken = useRevokeToken()

  const consequences: [string, ...string[]] = [
    t('confirmAction.token.consequence0', {
      defaultValue: 'Anything using this token loses access immediately.',
    }),
    t('confirmAction.token.consequence1', {
      defaultValue: 'This cannot be undone.',
    }),
  ]
  if (token.abilities.length > 0) {
    consequences.push(
      t('confirmAction.token.abilities', {
        defaultValue: 'Abilities: {{abilities}}',
        abilities: token.abilities.join(', '),
      }),
    )
  }

  if (token.dev_fixture) {
    consequences.push(
      t('confirmAction.token.devFixture', {
        defaultValue:
          'Dev fixture tokens are reseeded on the next dev-mode start unless you remove them from the fixtures file.',
      }),
    )
  }

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          {t('confirmAction.token.trigger', { defaultValue: 'Revoke' })}
        </Button>
      )}
      title={t('confirmAction.token.title', {
        defaultValue: 'Revoke "{{name}}"?',
        name: token.name,
      })}
      consequences={consequences}
      confirmLabel={t('confirmAction.token.confirm', {
        defaultValue: 'Revoke token',
      })}
      pendingLabel={t('confirmAction.token.pending', {
        defaultValue: 'Revoking...',
      })}
      pending={revokeToken.isPending}
      error={revokeToken.isError ? revokeToken.error.message : null}
      onOpenChange={(next) => {
        if (!next) revokeToken.reset()
      }}
      onConfirm={(close) => {
        revokeToken.mutate(token.id, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.token.success', {
                defaultValue: 'Token "{{name}}" revoked.',
                name: token.name,
              }),
              type: 'success',
            })
          },
        })
      }}
    />
  )
}
