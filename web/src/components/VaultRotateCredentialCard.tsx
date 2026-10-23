import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { formatDate } from '../lib/format'
import { useRotateVaultCredential, type VaultSettings } from '../queries/vault'
import { RotateValueDialog } from './env/RotateValueDialog'

export function VaultRotateCredentialCard({
  settings,
}: {
  settings: VaultSettings
}) {
  const { t } = useTranslation('secretRotation')
  const [open, setOpen] = useState(false)
  const rotate = useRotateVaultCredential()

  if (!settings.has_credential) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('vault.title')}</CardTitle>
        <CardDescription>{t('vault.description')}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {settings.credential_rotated_at
            ? t('rotateValue.lastRotated', {
                date: formatDate(
                  settings.credential_rotated_at,
                  t('unknownDate'),
                ),
              })
            : t('rotateValue.neverRotated')}
          {settings.previous_expires_at
            ? ` · ${t('rotateValue.graceUntil', {
                date: formatDate(
                  settings.previous_expires_at,
                  t('unknownDate'),
                ),
              })}`
            : ''}
        </p>
        <Button
          type="button"
          variant="outline"
          writeAction
          onClick={() => {
            setOpen(true)
          }}
        >
          {t('vault.button')}
        </Button>
      </CardContent>
      <RotateValueDialog
        open={open}
        onOpenChange={(next) => {
          if (!next) rotate.reset()
          setOpen(next)
        }}
        title={t('vault.dialogTitle')}
        description={t('vault.dialogDescription')}
        consequences={[t('vault.consequenceKeep'), t('vault.consequenceAudit')]}
        valueLabel={t('vault.valueLabel')}
        valueRequired
        pending={rotate.isPending}
        error={rotate.error ? rotate.error.message : null}
        onSubmit={(value) => {
          rotate.mutate(value, {
            onSuccess: () => {
              setOpen(false)
              toast.add({ title: t('vault.toast'), type: 'success' })
            },
          })
        }}
      />
    </Card>
  )
}
