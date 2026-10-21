import { useTranslation } from 'react-i18next'
import { TerminalWindowIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { openCliModal } from '../../lib/cliModalStore'
import { useCliTokens } from '../../queries/myCredentials'

// Renders nothing until the credentials are known, and nothing when the
// query fails (no permission to read them), so it never flashes or nags.
export function CliStatusBadge() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const { data } = useCliTokens()
  if (!data || data.length > 0) {
    return null
  }
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="relative rounded-full"
      aria-label={t('cliModal.badgeLabel')}
      onClick={openCliModal}
    >
      <TerminalWindowIcon aria-hidden="true" />
      <span className="hidden @3xl:inline">{t('cliModal.badge')}</span>
      <span
        aria-hidden="true"
        data-testid="cli-badge-dot"
        className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-amber-500 ring-2 ring-background"
      />
    </Button>
  )
}
