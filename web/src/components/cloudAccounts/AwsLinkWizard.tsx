import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { useIsRoot } from '../../hooks/useIsRoot'
import { cloudAccountActions } from '../../lib/cloudAccess'
import {
  AWS_STEPS,
  INITIAL_WIZARD,
  canLeaveStep,
  nextStep,
  parseRegions,
  prevStep,
  scopeIssues,
  type AwsStep,
  type AwsWizardState,
} from '../../lib/cloudAccounts'
import {
  useAwsLinkKit,
  useCreateAwsRoleAccount,
  useProbeCloudAccount,
  useVerifyCloudAccount,
} from '../../queries/cloudAccounts'
import type {
  AwsLinkKit,
  CloudAccount,
  CloudCapabilityResult,
  CloudVerifyResult,
} from '../../types/cloudAccounts'
import { AwsLinkConnectStep } from './AwsLinkConnectStep'
import { AwsLinkKitStep } from './AwsLinkKitStep'
import { AwsLinkScopeStep } from './AwsLinkScopeStep'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function AwsLinkWizard() {
  const { t } = useTranslation('cloudAccounts')
  const navigate = useNavigate()
  const actions = cloudAccountActions(useIsRoot())
  const [step, setStep] = useState<AwsStep>('scope')
  const [state, setState] = useState<AwsWizardState>(INITIAL_WIZARD)
  const [showIssues, setShowIssues] = useState(false)
  const [kit, setKit] = useState<AwsLinkKit | null>(null)
  const [account, setAccount] = useState<CloudAccount | null>(null)
  const [verify, setVerify] = useState<CloudVerifyResult | null>(null)
  const [probe, setProbe] = useState<CloudCapabilityResult[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const makeKit = useAwsLinkKit()
  const create = useCreateAwsRoleAccount()
  const verifyAccount = useVerifyCloudAccount()
  const probeAccount = useProbeCloudAccount()
  const busy =
    makeKit.isPending ||
    create.isPending ||
    verifyAccount.isPending ||
    probeAccount.isPending

  const patch = (p: Partial<AwsWizardState>) => {
    setState((s) => ({ ...s, ...p }))
    // Any scope change invalidates the generated kit and its external id.
    if (step === 'scope') setKit(null)
  }

  function generate() {
    if (scopeIssues(state).length > 0) {
      setShowIssues(true)
      return
    }
    setError(null)
    makeKit.mutate(
      {
        capabilities: [...state.capabilities],
        regions: parseRegions(state.regionsText),
        trusted_principal_arn: state.trustedPrincipalArn.trim(),
        role_name: state.roleName || undefined,
        guard_policy_arn: state.guardPolicyArn.trim() || undefined,
        region: state.region,
      },
      {
        onSuccess: (k) => {
          setKit(k)
          setStep('kit')
        },
        onError: (e) => {
          setError(messageOf(e))
        },
      },
    )
  }

  async function connect() {
    if (!kit) return
    setError(null)
    try {
      const created =
        account ??
        (await create.mutateAsync({
          label: state.label.trim(),
          role_arn: state.roleArn.trim(),
          external_id: kit.external_id,
          region: state.region,
        }))
      setAccount(created)
      const v = await verifyAccount.mutateAsync(created.id)
      setVerify(v)
      if (!v.ok) return
      const p = await probeAccount.mutateAsync({
        id: created.id,
        capabilities: [...state.capabilities],
      })
      setProbe(p.capabilities)
    } catch (e) {
      setError(messageOf(e))
    }
  }

  const onConnect = step === 'connect'
  const idx = AWS_STEPS.indexOf(step)

  return (
    <PageTemplate
      icon={<CloudGlassIcon size="1.4em" />}
      title={t('wizard.title')}
      description={t('wizard.description')}
      width="wide"
    >
      {actions.create ? (
        <div className="space-y-4">
          <ol
            aria-label={t('wizard.steps')}
            className="flex flex-wrap gap-x-4 gap-y-1 text-sm"
          >
            {AWS_STEPS.map((s, i) => (
              <li
                key={s}
                aria-current={s === step ? 'step' : undefined}
                className={
                  s === step
                    ? 'font-semibold text-foreground'
                    : 'text-muted-foreground'
                }
              >
                {i + 1}. {t(`wizard.stepNames.${s}`)}
              </li>
            ))}
          </ol>
          {step === 'scope' ? (
            <AwsLinkScopeStep
              state={state}
              onChange={patch}
              showIssues={showIssues}
            />
          ) : null}
          {step === 'kit' && kit ? <AwsLinkKitStep kit={kit} /> : null}
          {onConnect ? (
            <AwsLinkConnectStep
              state={state}
              onChange={patch}
              locked={account !== null}
              verify={verify}
              probe={probe}
              error={error}
            />
          ) : error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={idx === 0 || busy || account !== null}
              onClick={() => {
                setStep(prevStep(step))
              }}
            >
              {t('wizard.back')}
            </Button>
            <div className="flex gap-2">
              {probe && account ? (
                <Button
                  size="sm"
                  onClick={() => {
                    void navigate({
                      to: '/integrations/cloud-accounts/$id',
                      params: { id: account.id },
                    })
                  }}
                >
                  {t('wizard.openAccount')}
                </Button>
              ) : null}
              {step === 'scope' ? (
                <Button size="sm" disabled={busy} onClick={generate}>
                  {kit ? t('wizard.regenerate') : t('wizard.generate')}
                </Button>
              ) : null}
              {step === 'kit' ? (
                <Button
                  size="sm"
                  disabled={!canLeaveStep('kit', state, kit !== null)}
                  onClick={() => {
                    setStep(nextStep(step))
                  }}
                >
                  {t('wizard.kit.created')}
                </Button>
              ) : null}
              {onConnect && !probe ? (
                <Button
                  size="sm"
                  disabled={busy || !canLeaveStep('connect', state, true)}
                  onClick={() => {
                    void connect()
                  }}
                >
                  {account ? t('wizard.connect.retry') : t('wizard.connect.go')}
                </Button>
              ) : null}
            </div>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('wizard.needsAdmin')}{' '}
          <Link to="/integrations" className="underline">
            {t('common.backToIntegrations')}
          </Link>
        </p>
      )}
    </PageTemplate>
  )
}
