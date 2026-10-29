import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import type { OAuthSSOPolicy, SSOTestResult } from '@/queries/ssoOidc'
import { fetchSSOTestResult, policyInput } from '@/queries/ssoOidc'
import type { RoleResource } from '@/queries/roles'
import { brandKeyFor } from '@/lib/oauthProviders'
import { BrandTile } from '@/components/BrandLogoBadge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { FormSection } from '@/components/ui/form-section'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { OIDCPolicyForm } from './OIDCPolicyForm'
import { OIDCTestResult } from './OIDCTestResult'

const TEST_PARAM = 'sso_test'

// The test login ends on this page with the provider in the query string.
function useReturnedTest(): { provider: string | null; clear: () => void } {
  const [provider, setProvider] = useState<string | null>(() => {
    try {
      const v = new URLSearchParams(window.location.search).get(TEST_PARAM)
      return v?.startsWith('saml:') ? null : v
    } catch {
      return null
    }
  })
  return {
    provider,
    clear: () => {
      setProvider(null)
      const url = new URL(window.location.href)
      url.searchParams.delete(TEST_PARAM)
      window.history.replaceState(null, '', url)
    },
  }
}

/** OIDCPolicyList lists OAuth and OIDC providers with their SSO policy. */
export function OIDCPolicyList({
  policies,
  roles,
}: {
  policies: OAuthSSOPolicy[]
  roles: RoleResource[]
}) {
  const { t } = useTranslation('sso')
  const returned = useReturnedTest()
  const [editing, setEditing] = useState<string | null>(returned.provider)
  const [result, setResult] = useState<SSOTestResult | null>(null)
  const current = policies.find((p) => p.provider === editing)

  useEffect(() => {
    if (!returned.provider) return
    let live = true
    fetchSSOTestResult(returned.provider)
      .then((r) => {
        if (live && r.status !== 'none') setResult(r)
      })
      .catch(() => {
        if (live)
          setResult({ status: 'failed', failure_code: 'internal_error' })
      })
    return () => {
      live = false
    }
    // The result is one-shot: read it once for the provider the page returned with.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <FormSection title={t('oidc.title')} description={t('oidc.description')}>
      {policies.length === 0 ? (
        <EmptyState
          icon={<KeyGlassIcon className="size-5.5" />}
          title={t('oidc.emptyTitle')}
          description={t('oidc.emptyBody')}
          compact
        />
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border bg-card px-4">
          {policies.map((p) => (
            <li
              key={p.provider}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3"
            >
              <BrandTile
                name={brandKeyFor(p.provider)}
                label={p.name}
                size="sm"
                className="size-6 rounded"
              />
              <span className="min-w-0 flex-1 truncate text-sm font-medium">
                {p.name}
              </span>
              <Badge variant={p.enabled ? 'success' : 'muted'}>
                {p.enabled ? t('oidc.state.enabled') : t('oidc.state.off')}
              </Badge>
              {p.jit_provisioning ? (
                <Badge variant="outline">{t('list.jit')}</Badge>
              ) : null}
              {p.sync_roles ? (
                <Badge variant="outline">{t('oidc.syncBadge')}</Badge>
              ) : null}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setEditing(p.provider)}
              >
                <SettingsGlassIcon size="1.2em" aria-hidden="true" />
                {t('list.configure')}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <Sheet
        open={current !== undefined}
        onOpenChange={(open) => {
          if (!open) {
            setEditing(null)
            setResult(null)
            returned.clear()
          }
        }}
      >
        <SheetContent className="data-[side=right]:sm:max-w-xl">
          <SheetHeader className="border-b border-border pr-12">
            <SheetTitle>
              {t('oidc.form.title', { name: current?.name ?? '' })}
            </SheetTitle>
            <SheetDescription>
              {t('oidc.form.sheetDescription')}
            </SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 space-y-6 overflow-y-auto px-4 pb-4">
            {current && result ? (
              <OIDCTestResult result={result} policy={current} roles={roles} />
            ) : null}
            {current ? (
              <OIDCPolicyForm
                key={current.provider + JSON.stringify(policyInput(current))}
                policy={current}
                roles={roles}
              />
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
    </FormSection>
  )
}
