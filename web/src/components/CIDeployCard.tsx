import { useState } from 'react'
import { CodeBlock } from '@/components/ui/code-block'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  CopyIcon,
  RocketLaunchIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertAction, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { InfoTip } from '@/components/kit'
import {
  ciDeployKeys,
  useAddCITrust,
  useCITrusts,
  useCIWorkflow,
  useRemoveCITrust,
} from '../queries/ciDeploy'
import { useUpdateDashboardUrl } from '../queries/dashboardUrl'

const DEFAULT_REF = 'refs/heads/main'

function TrustForm({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const add = useAddCITrust(appName)
  const [repository, setRepository] = useState('')
  const [refPattern, setRefPattern] = useState(DEFAULT_REF)
  const [environment, setEnvironment] = useState('')

  function submit() {
    add.mutate(
      { repository, ref_pattern: refPattern, environment },
      {
        onSuccess: () => {
          setRepository('')
          toast.add({ title: t('ciDeploy.toast.trusted'), type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: t('ciDeploy.toast.trustError'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <div className="grid gap-2 sm:grid-cols-[2fr_2fr_1fr_auto]">
      <Input
        value={repository}
        placeholder={t('ciDeploy.repositoryPlaceholder')}
        aria-label={t('ciDeploy.repositoryLabel')}
        onChange={(e) => {
          setRepository(e.target.value)
        }}
      />
      <Input
        value={refPattern}
        aria-label={t('ciDeploy.refLabel')}
        onChange={(e) => {
          setRefPattern(e.target.value)
        }}
      />
      <Input
        value={environment}
        placeholder={t('ciDeploy.environmentPlaceholder')}
        aria-label={t('ciDeploy.environmentLabel')}
        onChange={(e) => {
          setEnvironment(e.target.value)
        }}
      />
      <Button
        type="button"
        onClick={submit}
        disabled={repository.trim() === '' || add.isPending}
      >
        {t('ciDeploy.trust')}
      </Button>
    </div>
  )
}

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
          <span className="min-w-0 truncate">
            <span className="font-medium">{trust.repository}</span>
            <span className="text-muted-foreground">
              {' '}
              {trust.ref_pattern}
              {trust.environment !== '' &&
                ` (${t('ciDeploy.environmentLabel')}: ${trust.environment})`}
            </span>
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
            <TrashIcon className="size-4" />
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
      <WarningIcon />
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

function WorkflowPanel({ appName }: { appName: string }) {
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
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-medium">{t('ciDeploy.workflowTitle')}</p>
        <Button type="button" variant="outline" size="sm" onClick={copy}>
          <CopyIcon className="size-4" />
          {t('ciDeploy.copy')}
        </Button>
      </div>
      <CodeBlock
        code={workflow.data.yaml}
        language="yaml"
        label={t('ciDeploy.workflowTitle')}
        maxHeight="lg"
      />
      <Collapsible>
        <CollapsibleTrigger className="text-xs text-muted-foreground hover:text-foreground">
          {t('ciDeploy.cliToggle')}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <CodeBlock
            className="mt-2"
            code={`levelrail-cli apps github-actions trust add ${appName} --repo owner/name\nlevelrail-cli apps github-actions workflow ${appName}`}
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
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RocketLaunchIcon className="size-4 text-muted-foreground" />
          {t('ciDeploy.title')}
          <InfoTip label={t('ciDeploy.infoTipLabel')}>
            {t('ciDeploy.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <TrustList appName={appName} />
        <TrustForm appName={appName} />
        <WorkflowPanel appName={appName} />
      </CardContent>
    </Card>
  )
}
