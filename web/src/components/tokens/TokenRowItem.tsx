import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { BotGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { cn } from '@/lib/utils'
import { RelativeTime } from '../kit/RelativeTime'
import { RevokeTokenDialog } from '../RevokeTokenDialog'
import { isPlatformToken, TOKEN_ABILITY_BADGE_VARIANT } from '../../types/token'
import type { ServedToken } from '../../types/tokenList'

// Same grid for the header and every row so columns line up from md up; below it rows stack.
export const ROW_GRID =
  'md:grid md:grid-cols-[1.25rem_minmax(0,2fr)_minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,2.2fr)] md:items-center md:gap-3'

const STATE = {
  active: 'healthy',
  expired: 'expired',
  revoked: 'stopped',
} as const

export function TokenListHeader() {
  const { t } = useTranslation('tokenList')
  return (
    <div
      aria-hidden="true"
      className={cn(
        'hidden border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground',
        ROW_GRID,
        'md:grid',
      )}
    >
      <span />
      <span>{t('columns.name')}</span>
      <span>{t('columns.abilities')}</span>
      <span>{t('columns.created')}</span>
      <span>{t('columns.lastUsed')}</span>
      <span>{t('columns.expires')}</span>
      <span>{t('columns.status')}</span>
    </div>
  )
}

interface TokenRowItemProps {
  token: ServedToken
  selected: boolean
  onSelect: (id: string, shift: boolean) => void
  onRotate: (token: ServedToken) => void
}

// Memoized: with hundreds of rows loaded, a selection change must not re-render them all.
export const TokenRowItem = memo(function TokenRowItem({
  token,
  selected,
  onSelect,
  onRotate,
}: TokenRowItemProps) {
  const { t } = useTranslation('tokenList')
  const { t: ts } = useTranslation('settings')
  const revoked = token.status === 'revoked'
  const platform = isPlatformToken(token)
  return (
    <li
      data-status={token.status}
      className={cn(
        'flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-border/60 px-3 py-2.5 text-sm',
        ROW_GRID,
        selected && 'bg-primary/5',
        revoked && 'opacity-60',
      )}
    >
      <input
        type="checkbox"
        checked={selected}
        aria-label={t('selectRow', { name: token.name })}
        onClick={(e) => {
          onSelect(token.id, e.shiftKey)
        }}
        onChange={() => undefined}
        className="size-4 shrink-0 accent-(--glinr-accent-solid)"
      />
      <div className="flex min-w-0 flex-1 items-center gap-1.5 md:flex-none">
        {token.agent ? (
          <BotGlassIcon
            className="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        ) : null}
        <span className="truncate font-medium" title={token.name}>
          {token.name}
        </span>
      </div>
      <div className="order-last flex min-w-0 basis-full flex-wrap gap-1 md:order-none md:basis-auto">
        {token.abilities.map((ability) => (
          <Badge key={ability} variant={TOKEN_ABILITY_BADGE_VARIANT[ability]}>
            {ability}
          </Badge>
        ))}
      </div>
      <span className="text-muted-foreground">
        <RelativeTime at={token.created_at} />
      </span>
      <span className="text-muted-foreground">
        {token.last_used_at ? (
          <RelativeTime at={token.last_used_at} />
        ) : (
          t('never')
        )}
      </span>
      <span className="text-muted-foreground">
        {token.expires_at ? (
          <RelativeTime at={token.expires_at} />
        ) : (
          t('noExpiry')
        )}
      </span>
      <div className="ml-auto flex flex-wrap items-center gap-2 md:ml-0">
        <StatusBadge
          state={STATE[token.status]}
          label={t(`status.${token.status}`)}
        />
        {revoked ? null : (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={platform}
              aria-label={ts('tokens.rotateLabel', { name: token.name })}
              onClick={() => {
                onRotate(token)
              }}
            >
              {ts('tokens.rotate')}
            </Button>
            <RevokeTokenDialog token={token} />
          </>
        )}
      </div>
    </li>
  )
})
