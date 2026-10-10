import { KeyIcon, RobotIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('tokens.name')}</TableHead>
            <TableHead>{t('tokens.agent')}</TableHead>
            <TableHead>{t('tokens.abilities')}</TableHead>
            <TableHead>{t('tokens.created')}</TableHead>
            <TableHead>{t('tokens.lastUsed')}</TableHead>
            <TableHead>{t('tokens.expires')}</TableHead>
            <TableHead>{t('tokens.status')}</TableHead>
            <TableHead className="text-right">{t('tokens.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {tokens.map((token) => {
            const revoked = Boolean(token.revoked_at)
            return (
              <TableRow
                key={token.id}
                className={revoked ? 'opacity-60' : undefined}
              >
                <TableCell className="font-medium text-foreground">
                  {token.name}
                </TableCell>
                <TableCell>
                  {token.agent ? (
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
                    <span className="text-muted-foreground">
                      {t('tokens.none')}
                    </span>
                  )}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {token.abilities.map((ability) => (
                      <Badge
                        key={ability}
                        variant={TOKEN_ABILITY_BADGE_VARIANT[ability]}
                      >
                        {ability}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  <RelativeTime at={token.created_at} />
                </TableCell>
                <TableCell className="text-muted-foreground">
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
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {token.expires_at ? (
                    <RelativeTime at={token.expires_at} />
                  ) : (
                    t('tokens.never')
                  )}
                </TableCell>
                <TableCell>
                  {revoked ? (
                    <Badge variant="destructive">
                      {t('tokens.revoked')}{' '}
                      {token.revoked_at ? (
                        <RelativeTime at={token.revoked_at} />
                      ) : null}
                    </Badge>
                  ) : (
                    <Badge variant="success">
                      {isActiveNow(token.last_used_at)
                        ? t('tokens.activeNow')
                        : t('tokens.active')}
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  {revoked ? null : <RevokeTokenDialog token={token} />}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
