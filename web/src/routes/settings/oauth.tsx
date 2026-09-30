import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { oauthSettingsQueryOptions } from '../../queries/oauth'
import { OAuthProviderCard } from '../../components/OAuthProviderCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'

// AbilityRoot-gated server-side (PUT /api/v1/settings/oauth/{provider}),
// same precedent as /settings/ingress: configures Google/GitHub sign-in.
export const Route = createFileRoute('/settings/oauth')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(oauthSettingsQueryOptions()),
  component: OAuthSettingsPage,
  pendingComponent: OAuthSettingsSkeleton,
})

// Two provider cards (Google, GitHub today): a fixed approximation of
// the real settings.map grid, which only ever grows by adding another
// provider to the backend list.
function OAuthSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <div className="grid gap-4 sm:grid-cols-2">
        <SettingsCardSkeleton description={false} rows={3} />
        <SettingsCardSkeleton description={false} rows={3} />
      </div>
    </div>
  )
}

function OAuthSettingsPage() {
  const { data: settings } = useSuspenseQuery(oauthSettingsQueryOptions())

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <KeyIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">
            OAuth sign-in
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Let people sign in with Google or GitHub instead of a password.
            Every signed-in account has identical access, there are no roles
            yet.
          </p>
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {settings.map((s) => (
          <OAuthProviderCard key={s.provider} settings={s} />
        ))}
      </div>
    </div>
  )
}
