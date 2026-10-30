import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'

interface Props {
  address: string
  password: string
  onDismiss: () => void
}

/** Shows a generated mailbox password once. The parent drops it on dismiss. */
export function MailSecretReveal({ address, password, onDismiss }: Props) {
  const { t } = useTranslation('mailServer')
  const { copied, copy } = useCopyToClipboard()
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/40 p-2 text-xs"
    >
      <span className="font-medium">
        {t('mailboxes.reveal.title', { address })}
      </span>
      <code
        aria-label={t('mailboxes.reveal.label')}
        className="rounded bg-background px-1.5 py-0.5 font-mono"
      >
        {password}
      </code>
      <Button
        size="sm"
        variant="outline"
        className="gap-1"
        onClick={() => copy(password)}
      >
        {copied ? (
          <CheckGlassIcon size="1.1em" />
        ) : (
          <CopyGlassIcon size="1.1em" />
        )}
        {copied ? t('mailboxes.reveal.copied') : t('mailboxes.reveal.copy')}
      </Button>
      <span className="text-muted-foreground">
        {t('mailboxes.reveal.once')}
      </span>
      <Button size="sm" onClick={onDismiss}>
        {t('mailboxes.reveal.saved')}
      </Button>
    </div>
  )
}
