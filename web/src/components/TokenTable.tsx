import { useState } from 'react'
import {
  BotGlassIcon,
  ChevronDownGlassIcon,
  KeyGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { RotateCliTokenDialog } from './cli/RotateCliTokenDialog'
import { cliOrigin } from '../lib/cliConnection'
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

interface TokenTableProps {
  tokens: TokenResource[]
  // Ids of CLI tokens the caller owns and may rotate; omitted hides Rotate.
  rotatableIds?: ReadonlySet<string>
}

export function TokenTable({ tokens, rotatableIds }: TokenTableProps) {
  const { t } = useTranslation('settings')
  const real = tokens.filter((tok) => !tok.dev_fixture)
  const fixtures = tokens.filter((tok) => tok.dev_fixture)
  return (
    <div className="space-y-4">
      <TokenRows tokens={real} rotatableIds={rotatableIds} />
      {fixtures.length > 0 ? (
        <Collapsible className="rounded-lg border border-border px-4 py-3">
          <CollapsibleTrigger className="flex items-center justify-between gap-2 text-sm font-medium">
            <span className="flex items-center gap-2">
              {t('tokens.devFixtureGroup')}
              <Badge variant="muted">
                {t('tokens.devFixtureGroupCount', { count: fixtures.length })}
              </Badge>
            </span>
            <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
          </CollapsibleTrigger>
          <CollapsiblePanel className="space-y-3 pt-3">
            <p className="text-sm text-muted-foreground">
              {t('tokens.devFixtureGroupBody')}
            </p>
            <TokenRows tokens={fixtures} />
          </CollapsiblePanel>
        </Collapsible>
      ) : null}
    </div>
  )
}

function TokenRows({ tokens, rotatableIds }: TokenTableProps) {
  const { t } = useTranslation('settings')
  const [rotateTarget, setRotateTarget] = useState<TokenResource | null>(null)
  if (tokens.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center">
        <div className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <KeyGlassIcon className="size-6" />
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
        <div className="space-y-1">
          <span className="font-medium break-all text-foreground">
            {token.name}
          </span>
          {token.dev_fixture ? (
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge variant="muted">{t('tokens.devFixtureBadge')}</Badge>
              {token.abilities.includes('root') ? (
                <span className="inline-flex items-center gap-1 text-xs text-tone-danger">
                  <TriangleAlertGlassIcon
                    className="size-5 text-tone-warning"
                    aria-hidden="true"
                  />
                  {t('tokens.devFixtureRootWarning')}
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
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
            <BotGlassIcon
              className="size-5 text-muted-foreground"
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
      id: 'lastIp',
      header: t('tokens.lastIp'),
      priority: 3,
      cell: (token) => (
        <span className="font-mono text-xs text-muted-foreground">
          {token.last_ip ?? '-'}
        </span>
      ),
    },
    {
      id: 'server',
      header: t('tokens.server'),
      priority: 3,
      cell: (token) => (
        <span className="font-mono text-xs text-muted-foreground">
          {token.instance_id ? token.instance_id.slice(0, 12) : '-'}
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
    <>
      <ResponsiveTable
        rows={tokens}
        columns={columns}
        getRowId={(token) => token.id}
        ariaLabel={t('tokens.tableLabel')}
        getRowProps={(token) => ({
          className: token.revoked_at ? 'opacity-60' : undefined,
        })}
        renderActions={(token) =>
          token.revoked_at ? null : (
            <div className="flex gap-2">
              {rotatableIds?.has(token.id) && cliOrigin(token.name) ? (
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={t('tokens.rotateLabel', { name: token.name })}
                  onClick={() => {
                    setRotateTarget(token)
                  }}
                >
                  {t('tokens.rotate')}
                </Button>
              ) : null}
              <RevokeTokenDialog token={token} />
            </div>
          )
        }
      />
      {rotateTarget ? (
        <RotateCliTokenDialog
          token={rotateTarget}
          onClose={() => {
            setRotateTarget(null)
          }}
        />
      ) : null}
    </>
  )
}
