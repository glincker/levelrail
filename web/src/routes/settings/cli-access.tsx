import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  CopyIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  deviceAuthRequestsQueryOptions,
  useDeviceAuthRequests,
} from '../../queries/deviceAuth'
import { DeviceAuthRequestTable } from '../../components/DeviceAuthRequestTable'
import { HelpLink } from '../../components/HelpLink'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { FormSection } from '@/components/ui/form-section'
import { CliAccessRail } from '../../components/settings/CliAccessRail'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

// Matches the copy-row idiom GitSourceWebhookBanner already established
// (code block + copy button in a muted bordered row); kept local since
// this page is the only place showing a shell command rather than a
// webhook URL/secret.
function CopyCommand({ command }: { command: string }) {
  const { t } = useTranslation('settings')
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
      <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
        {command}
      </code>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          void navigator.clipboard.writeText(command).then(() => {
            setCopied(true)
            setTimeout(() => {
              setCopied(false)
            }, 2000)
          })
        }}
      >
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <CopyIcon aria-hidden="true" />
        )}
        {copied ? t('cliAccessPage.copied') : t('cliAccessPage.copy')}
      </Button>
    </div>
  )
}

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
  const { user_code: highlightUserCode } = Route.useSearch()
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
      <div>
        <FormSection
          step={1}
          title={t('cliAccessPage.steps.install.title')}
          description={t('cliAccessPage.steps.install.body')}
        >
          <CopyCommand command="curl -fsSL https://levelrail.com/install-cli.sh | sh" />
          <p className="text-xs text-muted-foreground">
            {t('cliAccessPage.steps.install.note')}
          </p>
        </FormSection>
        <FormSection
          step={2}
          title={t('cliAccessPage.steps.point.title')}
          description={t('cliAccessPage.steps.point.body')}
        >
          <CopyCommand command={`export APP_API_URL=${apiUrl}`} />
        </FormSection>
        <FormSection
          step={3}
          title={t('cliAccessPage.steps.login.title')}
          description={t('cliAccessPage.steps.login.body')}
        >
          <CopyCommand command="levelrail-cli auth login --device" />
        </FormSection>
      </div>

      <DeviceAuthRequestTable
        requests={requests}
        highlightUserCode={highlightUserCode}
      />
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
