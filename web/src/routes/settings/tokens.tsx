import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { tokenListQueryOptions } from '../../queries/tokens'
import { TokenTable } from '../../components/TokenTable'
import { CreateTokenDialog } from '../../components/CreateTokenDialog'

// Account-level, not scoped to one app, so it lives under
// routes/settings/ rather than routes/apps/, per TASKS.md's "Frontend:
// dashboard UI" task for API token management. Reachable directly at
// /settings/tokens; no nav link wires it in yet, see this route's own
// report for why (routes/__root.tsx had unrelated concurrent edits in
// flight when this was built, per CLAUDE.md 8's parallel-agent
// guidance not to step on live work in the same file).
//
// Typed loader primes the Query cache before render, same pattern
// routes/apps/index.tsx already established: the component below only
// ever reads that warm cache via useSuspenseQuery, never fetches in its
// own body (CLAUDE.md 7).
export const Route = createFileRoute('/settings/tokens')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(tokenListQueryOptions()),
  component: TokensPage,
})

function TokensPage() {
  const { data: tokens } = useSuspenseQuery(tokenListQueryOptions())

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            API tokens
          </h1>
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
            Scoped, revocable credentials for the CLI, CI, and MCP integrations.
            Session login only mints tokens: a token can never mint or revoke
            another token on its own behalf.
          </p>
        </div>
        <CreateTokenDialog />
      </div>
      <TokenTable tokens={tokens} />
    </div>
  )
}
