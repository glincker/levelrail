import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { RevokeTokenDialog } from './RevokeTokenDialog'
import type { Ability, TokenResource } from '../types/token'

// Revoked tokens are never hidden from this list (GET /api/v1/auth/tokens
// keeps returning them per tokens.go's own doc comment on
// handleListTokens), so "revoked" is a row state this table renders
// distinctly, not a filter it applies.

const ABILITY_BADGE_VARIANT: Record<
  Ability,
  'default' | 'outline' | 'destructive' | 'muted'
> = {
  read: 'outline',
  'read:sensitive': 'outline',
  write: 'default',
  deploy: 'default',
  root: 'destructive',
}

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

export function TokenTable({ tokens }: { tokens: TokenResource[] }) {
  if (tokens.length === 0) {
    return (
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        No tokens yet. Create one to get a CLI, CI, or MCP credential.
      </p>
    )
  }

  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Abilities</TableHead>
            <TableHead>Created</TableHead>
            <TableHead>Last used</TableHead>
            <TableHead>Expires</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Actions</TableHead>
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
                  <div className="flex flex-wrap gap-1">
                    {token.abilities.map((ability) => (
                      <Badge
                        key={ability}
                        variant={ABILITY_BADGE_VARIANT[ability]}
                      >
                        {ability}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(token.created_at, '')}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(token.last_used_at, 'Never')}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(token.expires_at, 'Never')}
                </TableCell>
                <TableCell>
                  {revoked ? (
                    <Badge variant="destructive">
                      Revoked {formatDate(token.revoked_at, '')}
                    </Badge>
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
  )
}
