import { useState } from 'react'
import { CodeBlock } from '@/components/ui/code-block'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  CopyGlassIcon,
  RocketGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertAction, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toast } from '@/components/ui/toast'
import { InfoTip } from '@/components/kit'
import {
  ciDeployKeys,
  useCITrusts,
  useCIWorkflow,
  useRemoveCITrust,
} from '../queries/ciDeploy'
import { useUpdateDashboardUrl } from '../queries/dashboardUrl'
import { ciProviderOf, jobSnippet, type CIProvider } from '../lib/ciProviders'
import { CIProviderPicker, CITrustForm } from './CITrustForm'

function TrustList({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const trusts = useCITrusts(appName)
  const remove = useRemoveCITrust(appName)

  if (trusts.data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('ciDeploy.noTrusts')}</p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-lg border">
      {trusts.data.map((trust) => (
        <li
          key={trust.id}
          className="flex items-center justify-between gap-3 px-3 py-2 text-sm"
        >
          <span className="min-w-0">
            <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
              <span className="rounded bg-muted px-1.5 py-0.5 text-xs">
                {t(`ciDeploy.providers.${ciProviderOf(trust.provider)}`)}
              </span>
              <span className="truncate font-medium">{trust.repository}</span>
              <span className="truncate text-muted-foreground">
                {trust.ref_pattern}
                {trust.environment !== '' &&
                  ` (${t(`ciDeploy.environmentLabel.${ciProviderOf(trust.provider)}`)}: ${trust.environment})`}
              </span>
            </span>
            {trust.issuer && (
              <span className="block truncate text-xs text-muted-foreground">
                {t('ciDeploy.issuerShown', { issuer: trust.issuer })}
              </span>
            )}
            {ciProviderOf(trust.provider) === 'gitlab' &&
              trust.protected_ref_only === false && (
                <span className="flex items-start gap-1 text-xs text-destructive">
                  <TriangleAlertGlassIcon className="mt-0.5 size-4 shrink-0 text-tone-warning" />
                  {t('ciDeploy.unprotectedWarning')}
                </span>
              )}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t('ciDeploy.removeTrust', {
              repository: trust.repository,
            })}
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate(trust.id, {
                onError: (error) => {
                  toast.add({
                    title: t('ciDeploy.toast.removeError'),
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            <TrashGlassIcon className="size-4.5" />
          </Button>
        </li>
      ))}
    </ul>
  )
}

function DashboardUrlFix({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const queryClient = useQueryClient()
  const update = useUpdateDashboardUrl()

  function applyAddress() {
    update.mutate(
      { dashboard_url: window.location.origin },
      {
        onSuccess: () => {
          void queryClient.invalidateQueries({
            queryKey: ciDeployKeys.workflow(appName),
          })
        },
        onError: (error) => {
          toast.add({
            title: t('ciDeploy.toast.addressError'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Alert>
      <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
      <AlertDescription>{t('ciDeploy.needsAddress')}</AlertDescription>
      <AlertAction>
        <Button
          type="button"
          size="sm"
          onClick={applyAddress}
          disabled={update.isPending}
        >
          {t('ciDeploy.useThisAddress', { origin: window.location.origin })}
        </Button>
      </AlertAction>
    </Alert>
  )
}

const cliRepo: Record<CIProvider, string> = {
  github: 'owner/name',
  gitlab: 'group/project',
  circleci: '<PROJECT_ID> --org-id <ORG_ID>',
}

function ProviderSnippet({
  provider,
  appName,
  apiUrl,
}: {
  provider: CIProvider
  appName: string
  apiUrl: string
}) {
  const { t } = useTranslation('deploys')
  const snippet = jobSnippet(
    provider,
    appName,
    apiUrl === '' ? '$LEVELRAIL_API_URL' : apiUrl,
  )
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">
        {t(`ciDeploy.snippetTitle.${provider}`)}
      </p>
      <CodeBlock
        code={snippet.code}
        language={snippet.language}
        label={t(`ciDeploy.snippetTitle.${provider}`)}
        maxHeight="md"
      />
      <p className="text-xs text-muted-foreground">
        {t('ciDeploy.snippetNote')}
      </p>
    </div>
  )
}

function WorkflowPanel({
  appName,
  provider,
}: {
  appName: string
  provider: CIProvider
}) {
  const { t } = useTranslation('deploys')
  const workflow = useCIWorkflow(appName)

  function copy() {
    navigator.clipboard.writeText(workflow.data.yaml).then(
      () => {
        toast.add({ title: t('ciDeploy.toast.copied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('ciDeploy.toast.copyFailed'), type: 'error' })
      },
    )
  }

  return (
    <div className="space-y-3">
      {workflow.data.needs_dashboard_url && (
        <DashboardUrlFix appName={appName} />
      )}
      {provider === 'github' ? (
        <>
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm font-medium">{t('ciDeploy.workflowTitle')}</p>
            <Button type="button" variant="outline" size="sm" onClick={copy}>
              <CopyGlassIcon className="size-4.5" />
              {t('ciDeploy.copy')}
            </Button>
          </div>
          <CodeBlock
            code={workflow.data.yaml}
            language="yaml"
            label={t('ciDeploy.workflowTitle')}
            maxHeight="lg"
          />
        </>
      ) : (
        <ProviderSnippet
          provider={provider}
          appName={appName}
          apiUrl={workflow.data.api_url}
        />
      )}
      <Collapsible>
        <CollapsibleTrigger className="text-xs text-muted-foreground hover:text-foreground">
          {t('ciDeploy.cliToggle')}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <CodeBlock
            className="mt-2"
            code={`levelrail-cli apps ci trust add ${appName} --provider ${provider} --repo ${cliRepo[provider]}\nlevelrail-cli apps ci workflow ${appName}`}
            language="bash"
            label={t('ciDeploy.cliToggle')}
            prompt
          />
        </CollapsiblePanel>
      </Collapsible>
    </div>
  )
}

// CIDeployCard generates the deploy workflow and manages which repositories
// may deploy this app through a short-lived OIDC token (no stored secret).
export function CIDeployCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const [provider, setProvider] = useState<CIProvider>('github')
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RocketGlassIcon className="size-4.5 text-muted-foreground" />
          {t('ciDeploy.title')}
          <InfoTip label={t('ciDeploy.infoTipLabel')}>
            {t('ciDeploy.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <TrustList appName={appName} />
        <CIProviderPicker provider={provider} onChange={setProvider} />
        <CITrustForm key={provider} appName={appName} provider={provider} />
        <WorkflowPanel appName={appName} provider={provider} />
      </CardContent>
    </Card>
  )
}
