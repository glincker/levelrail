import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { DatabaseCredential } from '../../types/databaseAccess'

function CopyField({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('databaseAccess')
  const [copied, setCopied] = useState(false)

  function copy() {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="flex items-start gap-2">
        <code className="min-w-0 flex-1 break-all rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
          {value}
        </code>
        <Button
          variant="outline"
          size="sm"
          onClick={copy}
          aria-label={`${t('credential.copy')} ${label}`}
        >
          {copied ? (
            <CheckIcon aria-hidden="true" />
          ) : (
            <CopyIcon aria-hidden="true" />
          )}
          {copied ? t('credential.copied') : t('credential.copy')}
        </Button>
      </div>
    </div>
  )
}

/** CredentialDialog shows a freshly issued login exactly once. Closing it discards the password for good. */
export function CredentialDialog({
  credential,
  onClose,
}: {
  credential: DatabaseCredential | null
  onClose: () => void
}) {
  const { t } = useTranslation('databaseAccess')
  return (
    <Dialog
      open={credential !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent>
        {credential ? (
          <>
            <DialogHeader>
              <DialogTitle>{t('credential.title')}</DialogTitle>
              <DialogDescription>
                {t('credential.description')}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              <CopyField
                label={t('credential.username')}
                value={credential.username}
              />
              <CopyField
                label={t('credential.password')}
                value={credential.password}
              />
              <CopyField
                label={t('credential.fromApps')}
                value={credential.internal_url}
              />
              {credential.external_url ? (
                <div className="space-y-1">
                  <CopyField
                    label={t('credential.fromOutside')}
                    value={credential.external_url}
                  />
                  <p className="text-xs text-muted-foreground">
                    {credential.external_note}
                  </p>
                </div>
              ) : null}
              {credential.expires_at ? (
                <p className="text-xs text-muted-foreground">
                  {t('credential.expires', {
                    when: new Date(credential.expires_at).toLocaleString(),
                  })}
                </p>
              ) : null}
              {credential.sslmode === 'require' ? (
                <p className="text-xs text-muted-foreground">
                  {t('credential.tlsNote')}
                </p>
              ) : null}
            </div>
            <DialogFooter>
              <Button onClick={onClose}>{t('credential.done')}</Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
