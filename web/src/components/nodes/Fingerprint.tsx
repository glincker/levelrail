import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { groupFingerprint } from '../../lib/nodeTrust'

const COPIED_RESET_MS = 2000

// A SHA-256 fingerprint in colon-grouped hex with a copy button. The raw
// hex is what gets copied, since that is what APP_CA_FINGERPRINT takes.
export function Fingerprint({ value }: { value?: string }) {
  const { t } = useTranslation('nodes')
  const [copied, setCopied] = useState(false)
  if (!value) {
    return (
      <span className="text-muted-foreground">{t('trust.unavailable')}</span>
    )
  }
  return (
    <span className="flex items-start gap-1.5">
      <code className="min-w-0 flex-1 font-mono text-xs break-all text-foreground">
        {groupFingerprint(value)}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        aria-label={t('trust.copyFingerprint')}
        onClick={() => {
          void navigator.clipboard.writeText(value).then(() => {
            setCopied(true)
            window.setTimeout(() => {
              setCopied(false)
            }, COPIED_RESET_MS)
          })
        }}
      >
        {copied ? (
          <CheckGlassIcon className="size-5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-5" aria-hidden="true" />
        )}
      </Button>
    </span>
  )
}
