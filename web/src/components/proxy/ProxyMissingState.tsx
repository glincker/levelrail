import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DownloadGlassIcon,
  InfoGlassIcon,
  LoaderGlassIcon,
  SettingsGlassIcon,
  UndoGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useInstallProxy,
  type ProxyInstallPlan,
} from '../../queries/proxyInstall'
import type {
  ProxyIntegration,
  ProxyProviderKind,
  ProxySettings,
} from '../../queries/proxyIntegration'
import { ProxyInstallPlanView } from './ProxyInstallPlanView'
import { ProxyManualFields } from './ProxyManualFields'

function Choice({
  icon,
  title,
  body,
  children,
}: {
  icon: React.ReactNode
  title: string
  body: string
  children: React.ReactNode
}) {
  return (
    <div className="flex min-w-0 flex-col gap-3 rounded-lg border border-border p-4">
      <div className="flex items-start gap-2">
        <span aria-hidden="true" className="mt-0.5 shrink-0">
          {icon}
        </span>
        <div className="min-w-0">
          <h4 className="text-sm font-medium text-foreground">{title}</h4>
          <p className="text-xs text-muted-foreground">{body}</p>
        </div>
      </div>
      <div className="mt-auto flex flex-wrap gap-2">{children}</div>
    </div>
  )
}

// Shown instead of a red error wall when the chosen managed proxy is not
// running: install it, go back to the built-in ingress, or enter it by hand.
export default function ProxyMissingState({
  kind,
  proxyName,
  data,
  canChange,
  busy,
  checking,
  onBackToBuiltIn,
  onInstalled,
  onSaveManual,
  onRecheck,
}: {
  kind: ProxyProviderKind
  proxyName: string
  data: ProxyIntegration
  canChange: boolean
  busy: boolean
  checking: boolean
  onBackToBuiltIn: () => void
  onInstalled: (plan: ProxyInstallPlan) => void
  onSaveManual: (settings: ProxySettings) => void
  onRecheck: () => void
}) {
  const { t } = useTranslation('domains')
  const install = useInstallProxy()
  const [plan, setPlan] = useState<ProxyInstallPlan | null>(null)
  const [refusal, setRefusal] = useState('')
  const [manual, setManual] = useState(false)
  const canInstall = kind === 'traefik'
  const found = data.detected.kind

  const showPlan = () => {
    setRefusal('')
    install.mutate(
      { provider: 'traefik', confirm: false },
      {
        onSuccess: (res) => setPlan(res.plan),
        onError: (e) => {
          setPlan(null)
          setRefusal(e.message)
        },
      },
    )
  }
  const confirmInstall = () => {
    install.mutate(
      { provider: 'traefik', confirm: true },
      {
        onSuccess: (res) => {
          if (res.outcome === 'fixed' || res.outcome === 'skipped') {
            setPlan(null)
            toast.add({ title: t('proxyMissing.installed'), type: 'success' })
            onInstalled(res.plan)
            return
          }
          setRefusal(res.detail ?? t('proxyMissing.installFailed'))
        },
        onError: (e) => {
          setPlan(null)
          setRefusal(e.message)
        },
      },
    )
  }
  const loadingPlan = install.isPending && !install.variables?.confirm

  return (
    <div className="min-w-0 space-y-4">
      <div className="flex min-w-0 items-start gap-3 rounded-lg border border-border bg-muted/30 p-4">
        <InfoGlassIcon
          className="mt-0.5 size-6 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="min-w-0 space-y-1">
          <h3 className="text-sm font-medium text-foreground">
            {t('proxyMissing.title', { proxy: proxyName })}
          </h3>
          <p className="text-xs text-muted-foreground">
            {found === 'none'
              ? t('proxyMissing.looked', { proxy: proxyName })
              : t('proxyMissing.foundOther', {
                  proxy: proxyName,
                  other: t(`proxyPicker.provider.${found}`),
                })}
          </p>
        </div>
      </div>
      {refusal ? (
        <Alert>
          <InfoGlassIcon className="text-tone-info" size="1.4em" />
          <AlertTitle>{t('proxyMissing.refusedTitle')}</AlertTitle>
          <AlertDescription>
            <p className="break-words">{refusal}</p>
          </AlertDescription>
        </Alert>
      ) : null}
      {plan ? (
        <ProxyInstallPlanView
          plan={plan}
          applying={install.isPending}
          onConfirm={confirmInstall}
          onCancel={() => setPlan(null)}
        />
      ) : null}
      <div className="grid min-w-0 gap-3 lg:grid-cols-3">
        <Choice
          icon={<DownloadGlassIcon className="size-6" />}
          title={t('proxyMissing.install.title', { proxy: proxyName })}
          body={
            canInstall
              ? t('proxyMissing.install.body', { proxy: proxyName })
              : t('proxyMissing.install.unsupported', { proxy: proxyName })
          }
        >
          {canInstall ? (
            <Button
              type="button"
              disabled={!canChange || busy || install.isPending}
              onClick={showPlan}
            >
              {loadingPlan ? (
                <LoaderGlassIcon
                  size="1.4em"
                  className="animate-spin"
                  aria-hidden="true"
                />
              ) : null}
              {t('proxyMissing.install.action', { proxy: proxyName })}
            </Button>
          ) : null}
        </Choice>
        <Choice
          icon={<UndoGlassIcon className="size-6" />}
          title={t('proxyMissing.builtin.title')}
          body={t('proxyMissing.builtin.body')}
        >
          <Button
            type="button"
            variant="outline"
            disabled={!canChange || busy}
            onClick={onBackToBuiltIn}
          >
            {t('proxyMissing.builtin.action')}
          </Button>
        </Choice>
        <Choice
          icon={<SettingsGlassIcon className="size-6" />}
          title={t('proxyMissing.elsewhere.title')}
          body={t('proxyMissing.elsewhere.body')}
        >
          <Button
            type="button"
            variant="outline"
            aria-expanded={manual}
            onClick={() => setManual((open) => !open)}
          >
            {manual
              ? t('proxyMissing.elsewhere.hide')
              : t('proxyMissing.elsewhere.action')}
          </Button>
        </Choice>
      </div>
      {manual ? (
        <ProxyManualFields
          kind={kind}
          base={data.settings}
          pending={busy || !canChange}
          checking={checking}
          onSave={onSaveManual}
          onRecheck={onRecheck}
        />
      ) : null}
    </div>
  )
}
