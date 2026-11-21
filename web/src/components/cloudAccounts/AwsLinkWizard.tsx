import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { useCloudAccountActions } from '../../hooks/useCloudAccountActions'
import {
  AWS_STEPS,
  INITIAL_WIZARD,
  canLeaveStep,
  nextStep,
  parseRegions,
  parseZones,
  prevStep,
  scopeIssues,
  type AwsStep,
  type AwsWizardState,
} from '../../lib/cloudAccounts'
import { principalFromIdentity } from '../../lib/cloudSpend'
import {
  useAwsControlIdentity,
  useAwsLinkKit,
  useAwsVaultKit,
  useCreateAwsRoleAccount,
  useCreateAwsVaultAccount,
  useProbeCloudAccount,
  useVerifyCloudAccount,
} from '../../queries/cloudAccounts'
import type {
  AwsLinkKit,
  AwsVaultKit,
  CloudAccount,
  CloudCapabilityResult,
  CloudVerifyResult,
} from '../../types/cloudAccounts'
import { AwsLinkConnectStep } from './AwsLinkConnectStep'
import { AwsLinkKitStep } from './AwsLinkKitStep'
import { AwsLinkScopeStep } from './AwsLinkScopeStep'
import { AwsLinkVaultKitStep } from './AwsLinkVaultKitStep'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function AwsLinkWizard() {
  const { t } = useTranslation('cloudAccounts')
  const navigate = useNavigate()
  const actions = useCloudAccountActions()
  const [step, setStep] = useState<AwsStep>('scope')
  const identity = useAwsControlIdentity(actions.create)
  const [raw, setState] = useState<AwsWizardState>(INITIAL_WIZARD)
  const [principalEdited, setPrincipalEdited] = useState(false)
  // Prefill is derived, so a user edit (even to empty) is never overwritten.
  const prefill = principalFromIdentity(identity.data)
  const state: AwsWizardState = principalEdited
    ? raw
    : { ...raw, trustedPrincipalArn: prefill }
  const [showIssues, setShowIssues] = useState(false)
  const [kit, setKit] = useState<AwsLinkKit | null>(null)
  const [vaultKit, setVaultKit] = useState<AwsVaultKit | null>(null)
  const [account, setAccount] = useState<CloudAccount | null>(null)
  const [verify, setVerify] = useState<CloudVerifyResult | null>(null)
  const [probe, setProbe] = useState<CloudCapabilityResult[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const makeKit = useAwsLinkKit()
  const create = useCreateAwsRoleAccount()
  const makeVaultKit = useAwsVaultKit()
  const createVault = useCreateAwsVaultAccount()
  const verifyAccount = useVerifyCloudAccount()
  const probeAccount = useProbeCloudAccount()
  const busy =
    makeKit.isPending ||
    makeVaultKit.isPending ||
    create.isPending ||
    createVault.isPending ||
    verifyAccount.isPending ||
    probeAccount.isPending

  const patch = (p: Partial<AwsWizardState>) => {
    if (p.trustedPrincipalArn !== undefined) setPrincipalEdited(true)
    setState((s) => ({ ...s, ...p }))
    // Any scope change invalidates the generated kit and its external id.
    if (step === 'scope') {
      setKit(null)
      setVaultKit(null)
    }
  }

  function generate() {
    if (scopeIssues(state).length > 0) {
      setShowIssues(true)
      return
    }
    setError(null)
    if (state.useVault) {
      makeVaultKit.mutate(
        {
          account_id: state.accountId.trim(),
          vault_principal_arn: state.trustedPrincipalArn.trim(),
          capabilities: [...state.capabilities],
          regions: parseRegions(state.regionsText),
          hosted_zone_ids: parseZones(state.zonesText),
          role_name: state.roleName || undefined,
          region: state.region,
          vault_mount: state.vaultMount || undefined,
          vault_role: state.vaultRole,
          vault_namespace: state.vaultNamespace || undefined,
        },
        {
          onSuccess: (k) => {
            setVaultKit(k)
            setStep('kit')
          },
          onError: (e) => {
            setError(messageOf(e))
          },
        },
      )
      return
    }
    makeKit.mutate(
      {
        capabilities: [...state.capabilities],
        regions: parseRegions(state.regionsText),
        hosted_zone_ids: parseZones(state.zonesText),
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
    if (!kit && !vaultKit) return
    setError(null)
    try {
      const created =
        account ??
        (kit && !state.useVault
          ? await create.mutateAsync({
              label: state.label.trim(),
              role_arn: state.roleArn.trim(),
              external_id: kit.external_id,
              region: state.region,
            })
          : await createVault.mutateAsync({
              label: state.label.trim(),
              region: state.region,
              account_id: state.accountId.trim(),
              vault_address: state.vaultAddress.trim(),
              vault_mount: state.vaultMount || undefined,
              vault_role: state.vaultRole.trim(),
              vault_namespace: state.vaultNamespace || undefined,
              vault_token_ref: state.vaultTokenRef.trim(),
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
              identity={identity.data}
              state={state}
              onChange={patch}
              showIssues={showIssues}
            />
          ) : null}
          {step === 'kit' && kit && !state.useVault ? (
            <AwsLinkKitStep kit={kit} />
          ) : null}
          {step === 'kit' && vaultKit && state.useVault ? (
            <AwsLinkVaultKitStep kit={vaultKit} />
          ) : null}
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
                  {kit || vaultKit
                    ? t('wizard.regenerate')
                    : t('wizard.generate')}
                </Button>
              ) : null}
              {step === 'kit' ? (
                <Button
                  size="sm"
                  disabled={
                    !canLeaveStep(
                      'kit',
                      state,
                      kit !== null || vaultKit !== null,
                    )
                  }
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
