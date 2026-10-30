import { useTranslation } from 'react-i18next'
import { TerminalGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { openCliModal } from '../../lib/cliModalStore'
import { useCliTokens } from '../../queries/myCredentials'

// Always the way into the CLI command center once the credentials are known:
// amber while nothing is connected, green with a count once something is.
// Renders nothing while loading or when the query fails (no permission to
// read them), so it never flashes or nags.
export function CliStatusBadge() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const { data } = useCliTokens()
  if (!data) {
    return null
  }
  const connected = data.length
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="relative rounded-full"
      aria-label={
        connected > 0
          ? t('cliModal.badgeLabelConnected', { count: connected })
          : t('cliModal.badgeLabel')
      }
      onClick={openCliModal}
    >
      <TerminalGlassIcon size="1.4em" aria-hidden="true" />
      <span className="hidden @3xl:inline">
        {connected > 0
          ? t('cliModal.badgeConnected', { count: connected })
          : t('cliModal.badge')}
      </span>
      <span
        aria-hidden="true"
        data-testid="cli-badge-dot"
        data-state={connected > 0 ? 'connected' : 'none'}
        className={cn(
          'absolute -top-0.5 -right-0.5 size-2 rounded-full ring-2 ring-background',
          connected > 0 ? 'bg-emerald-500' : 'bg-amber-500',
        )}
      />
    </Button>
  )
}
