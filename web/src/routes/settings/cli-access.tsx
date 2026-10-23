import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { tokenListQueryOptions } from '../../queries/tokens'
import { useCliTokenRows } from '../../queries/myCredentials'
import { TokenTable } from '../../components/TokenTable'
import { CliServerCard } from '../../components/settings/CliServerCard'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  deviceAuthRequestsQueryOptions,
  useDeviceAuthRequests,
} from '../../queries/deviceAuth'
import { DeviceAuthRequestTable } from '../../components/DeviceAuthRequestTable'
import { HelpLink } from '../../components/HelpLink'
import { CommandBlock } from '@/components/ui/code-block'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { FormSection } from '@/components/ui/form-section'
import { CLI_INSTALL_COMMAND } from '../../lib/cliInstall'
import { CliAccessRail } from '../../components/settings/CliAccessRail'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

interface CliAccessSearch {
  user_code?: string
}

// A plain function, not zod, matching reset-password.tsx's own reasoning:
// validateSearch runs as part of eager route matching, so pulling zod in
// here for one optional string would undo autoCodeSplitting's work.
function validateCliAccessSearch(
  search: Record<string, unknown>,
): CliAccessSearch {
  const { user_code } = search
  return typeof user_code === 'string' ? { user_code } : {}
}

// Web half of "levelrail-cli auth login --device": the CLI prints a
// user_code and polls POST /api/v1/auth/device/token, this page is
// where an operator sees that same code and approves or denies it.
// device_auth.go's VerificationURIComplete already points the CLI's
// printed link at /settings/cli-access?user_code=..., which
// validateSearch reads to highlight the matching row.
export const Route = createFileRoute('/settings/cli-access')({
  validateSearch: validateCliAccessSearch,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(deviceAuthRequestsQueryOptions()),
  component: CliAccessPage,
  pendingComponent: CliAccessPending,
})

function CliAccessPage() {
  const { t } = useTranslation('settings')
  const { data: requests } = useDeviceAuthRequests()
  const { data: tokens } = useQuery(tokenListQueryOptions())
  const { user_code: highlightUserCode } = Route.useSearch()
  // Only the caller's own CLI tokens can be rotated: the new pairing
  // command signs in the caller, so it cannot replace someone else's.
  const { data: ownCliRows } = useCliTokenRows(true)
  const rotatableIds = useMemo(
    () => new Set((ownCliRows ?? []).map((r) => r.token.id)),
    [ownCliRows],
  )
  // Same origin the dashboard itself is served from: in production the
  // API and web UI are one embedded binary, so this is the address the CLI needs.
  const apiUrl = window.location.origin

  return (
    <PageTemplate
      icon={<TerminalWindowIcon />}
      eyebrow={t('cliAccessPage.eyebrow')}
      title={t('cliAccessPage.title')}
      description={t('cliAccessPage.description')}
      aside={<CliAccessRail pending={requests.length} apiUrl={apiUrl} />}
      asideLabel={t('cliAccessPage.rail.label')}
      actions={
        <HelpLink path="/cli-reference" label={t('cliAccessPage.help')} />
      }
      status={
        <Badge variant={requests.length > 0 ? 'warning' : 'muted'}>
          {t('cliAccessPage.waitingCount', { count: requests.length })}
        </Badge>
      }
      banners={
        requests.length > 0 ? (
          <Banner
            variant="warning"
            title={t('cliAccessPage.banner.title', { count: requests.length })}
          >
            {t('cliAccessPage.banner.body')}
          </Banner>
        ) : null
      }
    >
      <CliServerCard />

      <DeviceAuthRequestTable
        requests={requests}
        highlightUserCode={highlightUserCode}
      />

      <Collapsible className="rounded-lg border border-border px-4 py-3">
        <CollapsibleTrigger className="flex items-center justify-between gap-2 text-sm font-medium">
          {t('cliAccessPage.setup.toggle')}
          <CaretDownIcon className="size-4" aria-hidden="true" />
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-2 pt-3">
          <p className="text-sm text-muted-foreground">
            {t('cliAccessPage.setup.hint')}
          </p>
          <div>
            <FormSection
              step={1}
              title={t('cliAccessPage.steps.install.title')}
              description={t('cliAccessPage.steps.install.body')}
            >
              <CommandBlock code={CLI_INSTALL_COMMAND} />
              <p className="text-xs text-muted-foreground">
                {t('cliAccessPage.steps.install.note')}
              </p>
            </FormSection>
            <FormSection
              step={2}
              title={t('cliAccessPage.steps.point.title')}
              description={t('cliAccessPage.steps.point.body')}
            >
              <CommandBlock code={`export APP_API_URL=${apiUrl}`} />
            </FormSection>
            <FormSection
              step={3}
              title={t('cliAccessPage.steps.login.title')}
              description={t('cliAccessPage.steps.login.body')}
            >
              <CommandBlock code="levelrail-cli auth login --device" />
            </FormSection>
          </div>
        </CollapsiblePanel>
      </Collapsible>

      <section className="space-y-2">
        <h2 className="text-sm font-medium">
          {t('cliAccessPage.tokens.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('cliAccessPage.tokens.body')}
        </p>
        <TokenTable
          tokens={(tokens ?? []).filter((tok) => !tok.agent)}
          rotatableIds={rotatableIds}
        />
      </section>
    </PageTemplate>
  )
}

function CliAccessPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="cli-access" />
      <TableSkeleton columnCount={5} />
    </div>
  )
}
