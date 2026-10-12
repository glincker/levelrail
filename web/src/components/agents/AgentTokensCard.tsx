import { useTranslation } from 'react-i18next'
import { RobotIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { RelativeTime } from '../kit'
import { EmptyState } from '../ui/empty-state'
import { RevokeTokenDialog } from '../RevokeTokenDialog'
import { CreateAgentTokenDialog } from './CreateAgentTokenDialog'
import {
  TOKEN_ABILITY_BADGE_VARIANT,
  type TokenResource,
} from '../../types/token'

const COLUMNS: ResponsiveColumn<TokenResource>[] = [
  {
    id: 'agent',
    header: 'Agent',
    primary: true,
    cell: (token) => (
      <span className="font-medium text-foreground">{token.agent?.name}</span>
    ),
  },
  {
    id: 'token',
    header: 'Token',
    priority: 2,
    cell: (token) => (
      <span className="text-muted-foreground">{token.name}</span>
    ),
  },
  {
    id: 'abilities',
    header: 'Abilities',
    priority: 2,
    cell: (token) => (
      <div className="flex flex-wrap gap-1">
        {token.abilities.map((a) => (
          <Badge key={a} variant={TOKEN_ABILITY_BADGE_VARIANT[a]}>
            {a}
          </Badge>
        ))}
      </div>
    ),
  },
  {
    id: 'lastUsed',
    header: 'Last used',
    priority: 3,
    cell: (token) => (
      <span className="text-muted-foreground">
        {token.last_used_at ? (
          <RelativeTime at={token.last_used_at} live />
        ) : (
          'Never'
        )}
      </span>
    ),
  },
  {
    id: 'status',
    header: 'Status',
    cell: (token) =>
      token.revoked_at ? (
        <Badge variant="destructive">Revoked</Badge>
      ) : (
        <Badge variant="muted">Active</Badge>
      ),
  },
]

export function AgentTokensCard({ tokens }: { tokens: TokenResource[] }) {
  const { t } = useTranslation('common')
  const agentTokens = tokens.filter((tok) => tok.agent)
  return (
    <section className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-foreground">
            Agent tokens
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Tokens labeled as issued to an AI agent. Every change made with one
            is recorded under the agent&apos;s name in the audit log.
          </p>
        </div>
        <CreateAgentTokenDialog />
      </div>

      {agentTokens.length === 0 ? (
        <EmptyState
          icon={<RobotIcon />}
          title="No agent tokens yet"
          description="Create one with a preset scope, then give it to your agent through the connect snippet above."
        />
      ) : (
        <ResponsiveTable
          rows={agentTokens}
          columns={COLUMNS}
          getRowId={(token) => token.id}
          ariaLabel={t('tableLabels.agentTokens')}
          getRowProps={(token) => ({
            className: token.revoked_at ? 'opacity-60' : undefined,
          })}
          renderActions={(token) =>
            token.revoked_at ? null : <RevokeTokenDialog token={token} />
          }
        />
      )}
    </section>
  )
}
