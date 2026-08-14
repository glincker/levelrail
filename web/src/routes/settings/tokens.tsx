import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { tokenListQueryOptions } from '../../queries/tokens'
import { TokenTable } from '../../components/TokenTable'
import { CreateTokenDialog } from '../../components/CreateTokenDialog'
import { useBrand } from '../../hooks/useBrand'

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
  const brand = useBrand()

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <KeyIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              API tokens
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Scoped, revocable credentials for the CLI, CI, and MCP
              integrations. Session login only mints tokens: a token can never
              mint or revoke another token on its own behalf.
            </p>
            {/* Dokploy's API-keys screen links straight to /swagger from
                this same header (docs-local/research/competitor-onboarding-
                auth-ux.md finding 12): the moment someone has a credential
                is the moment they most want to know what to do with it.
                brand.DocsURL is a real field on the /api/v1/brand response
                (CLAUDE.md section 3), not an invented route, so this only
                renders when the running instance actually configured one. */}
            {brand.DocsURL ? (
              <a
                href={brand.DocsURL}
                target="_blank"
                rel="noreferrer"
                className="mt-1 inline-block text-sm text-primary underline underline-offset-4 hover:no-underline"
              >
                View API docs
              </a>
            ) : null}
          </div>
        </div>
        <CreateTokenDialog />
      </div>
      <TokenTable tokens={tokens} />
    </div>
  )
}
