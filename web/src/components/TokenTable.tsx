import { KeyIcon, RobotIcon } from '@phosphor-icons/react/dist/ssr'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { RelativeTime } from './kit/RelativeTime'
import { RevokeTokenDialog } from './RevokeTokenDialog'
import { TOKEN_ABILITY_BADGE_VARIANT } from '../types/token'
import type { TokenResource } from '../types/token'

// Revoked tokens are never hidden from this list (GET /api/v1/auth/tokens
// keeps returning them per tokens.go's own doc comment on
// handleListTokens), so "revoked" is a row state this table renders
// distinctly, not a filter it applies.

const ACTIVE_NOW_MS = 5 * 60 * 1000

function isActiveNow(lastUsed: string | undefined): boolean {
  return lastUsed ? Date.now() - Date.parse(lastUsed) < ACTIVE_NOW_MS : false
}

export function TokenTable({ tokens }: { tokens: TokenResource[] }) {
  const { t } = useTranslation('settings')
  if (tokens.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center">
        <div className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <KeyIcon className="size-5" />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">
            {t('tokens.emptyTitle')}
          </p>
          <p className="text-sm text-muted-foreground">
            {t('tokens.emptyBody')}
          </p>
        </div>
      </div>
    )
  }

  const columns: ResponsiveColumn<TokenResource>[] = [
    {
      id: 'name',
      primary: true,
      header: t('tokens.name'),
      cell: (token) => (
        <span className="font-medium break-all text-foreground">
          {token.name}
        </span>
      ),
    },
    {
      id: 'agent',
      header: t('tokens.agent'),
      priority: 3,
      cell: (token) =>
        token.agent ? (
          <span
            className="inline-flex items-center gap-1.5 text-foreground"
            title={token.agent.description}
          >
            <RobotIcon
              className="size-3.5 text-muted-foreground"
              aria-hidden="true"
            />
            {token.agent.name}
          </span>
        ) : (
          <span className="text-muted-foreground">{t('tokens.none')}</span>
        ),
    },
    {
      id: 'abilities',
      header: t('tokens.abilities'),
      priority: 2,
      cell: (token) => (
        <div className="flex flex-wrap gap-1">
          {token.abilities.map((ability) => (
            <Badge key={ability} variant={TOKEN_ABILITY_BADGE_VARIANT[ability]}>
              {ability}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      id: 'created',
      header: t('tokens.created'),
      priority: 3,
      cell: (token) => (
        <span className="text-muted-foreground">
          <RelativeTime at={token.created_at} />
        </span>
      ),
    },
    {
      id: 'lastUsed',
      header: t('tokens.lastUsed'),
      priority: 2,
      cell: (token) => (
        <span className="text-muted-foreground">
          {token.last_used_at ? (
            <span className="inline-flex items-center gap-1.5">
              {isActiveNow(token.last_used_at) ? (
                <span
                  className="size-1.5 rounded-full bg-green-500"
                  role="img"
                  aria-label={t('tokens.activeNow')}
                />
              ) : null}
              <RelativeTime at={token.last_used_at} live />
            </span>
          ) : (
            t('tokens.never')
          )}
        </span>
      ),
    },
    {
      id: 'expires',
      header: t('tokens.expires'),
      priority: 3,
      cell: (token) => (
        <span className="text-muted-foreground">
          {token.expires_at ? (
            <RelativeTime at={token.expires_at} />
          ) : (
            t('tokens.never')
          )}
        </span>
      ),
    },
    {
      id: 'status',
      header: t('tokens.status'),
      cell: (token) =>
        token.revoked_at ? (
          <Badge variant="destructive">
            {t('tokens.revoked')} <RelativeTime at={token.revoked_at} />
          </Badge>
        ) : (
          <Badge variant="success">
            {isActiveNow(token.last_used_at)
              ? t('tokens.activeNow')
              : t('tokens.active')}
          </Badge>
        ),
    },
  ]

  return (
    <ResponsiveTable
      rows={tokens}
      columns={columns}
      getRowId={(token) => token.id}
      ariaLabel={t('tokens.tableLabel')}
      getRowProps={(token) => ({
        className: token.revoked_at ? 'opacity-60' : undefined,
      })}
      renderActions={(token) =>
        token.revoked_at ? null : <RevokeTokenDialog token={token} />
      }
    />
  )
}
