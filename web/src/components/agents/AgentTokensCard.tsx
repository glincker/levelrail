import { RobotIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { RelativeTime } from '../kit'
import { EmptyState } from '../ui/empty-state'
import { RevokeTokenDialog } from '../RevokeTokenDialog'
import { CreateAgentTokenDialog } from './CreateAgentTokenDialog'
import {
  TOKEN_ABILITY_BADGE_VARIANT,
  type TokenResource,
} from '../../types/token'

export function AgentTokensCard({ tokens }: { tokens: TokenResource[] }) {
  const agentTokens = tokens.filter((t) => t.agent)
  return (
    <section className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex items-start justify-between gap-3">
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
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Agent</TableHead>
                <TableHead>Token</TableHead>
                <TableHead>Abilities</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {agentTokens.map((token) => {
                const revoked = Boolean(token.revoked_at)
                return (
                  <TableRow
                    key={token.id}
                    className={revoked ? 'opacity-60' : undefined}
                  >
                    <TableCell className="font-medium text-foreground">
                      {token.agent?.name}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {token.name}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {token.abilities.map((a) => (
                          <Badge
                            key={a}
                            variant={TOKEN_ABILITY_BADGE_VARIANT[a]}
                          >
                            {a}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {token.last_used_at ? (
                        <RelativeTime at={token.last_used_at} live />
                      ) : (
                        'Never'
                      )}
                    </TableCell>
                    <TableCell>
                      {revoked ? (
                        <Badge variant="destructive">Revoked</Badge>
                      ) : (
                        <Badge variant="muted">Active</Badge>
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
      )}
    </section>
  )
}
