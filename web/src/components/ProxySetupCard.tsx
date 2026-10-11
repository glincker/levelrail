import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugsIcon, SpinnerIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { shouldShowSetupCard } from '../lib/proxyIntegration'
import {
  useProxyIntegration,
  useSetupProxyIntegration,
  type ProxyIntegration,
} from '../queries/proxyIntegration'
import { HelpLink } from './HelpLink'
import { ProxyDetectedFacts } from './proxy/ProxyDetectedFacts'
import { ProxyDomainRow } from './proxy/ProxyDomainRow'
import { ProxySetupConfirm } from './proxy/ProxySetupConfirm'
import { ProxySteps } from './proxy/ProxySteps'

function QuietNote({ children }: { children: React.ReactNode }) {
  return <p className="px-1 text-xs text-muted-foreground">{children}</p>
}

// Domains page card. Hidden once every domain is live, except in the
// session that just ran setup so the result stays on screen.
export function ProxySetupCard() {
  const { t } = useTranslation('domains')
  const { data, isLoading, isError, restartPolling } = useProxyIntegration()
  const setup = useSetupProxyIntegration()
  const [confirming, setConfirming] = useState(false)
  const [ranSetup, setRanSetup] = useState(false)

  if (isLoading) return null
  if (isError) return <QuietNote>{t('proxySetup.loadFailed')}</QuietNote>
  if (data === null || data === undefined) {
    return <QuietNote>{t('proxySetup.unavailable')}</QuietNote>
  }
  if (data.detected.kind === 'none') {
    return (
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg border border-dashed border-border px-4 py-3 text-sm">
        <span className="font-medium text-foreground">
          {t('proxySetup.none.title')}
        </span>
        <span className="text-muted-foreground">
          {t('proxySetup.none.body')}
        </span>
        <HelpLink
          path="/domains-and-ingress"
          label={t('proxySetup.none.guide')}
          variant="inline"
        />
      </div>
    )
  }
  if (!shouldShowSetupCard(data) && !ranSetup) return null

  return (
    <SetupCardBody
      data={data}
      pending={setup.isPending}
      confirming={confirming}
      setConfirming={setConfirming}
      onConfirm={() => {
        setup.mutate(
          {
            confirm: true,
            dynamic_dir: data.detected.dynamic_dir || undefined,
          },
          {
            onSuccess: () => {
              setConfirming(false)
              setRanSetup(true)
              restartPolling()
              toast.add({ title: t('proxySetup.setupDone'), type: 'success' })
            },
            onError: (error) => {
              toast.add({
                title: t('proxySetup.setupFailed'),
                description: error.message,
                type: 'error',
              })
            },
          },
        )
      }}
    />
  )
}

function SetupCardBody({
  data,
  pending,
  confirming,
  setConfirming,
  onConfirm,
}: {
  data: ProxyIntegration
  pending: boolean
  confirming: boolean
  setConfirming: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation('domains')
  const blocked =
    !data.detected.complete || data.steps.some((s) => s.state === 'blocked')
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugsIcon className="size-4" aria-hidden="true" />
          {t('proxySetup.title')}
        </CardTitle>
        <CardDescription>{t('proxySetup.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="grid gap-6 lg:grid-cols-2">
          <ProxySteps steps={data.steps} />
          <ProxyDetectedFacts detected={data.detected} />
        </div>
        {data.domains.length > 0 ? (
          <div className="space-y-1">
            <h3 className="text-sm font-semibold text-foreground">
              {t('proxySetup.domains.title')}
            </h3>
            <ul className="divide-y divide-border">
              {data.domains.map((row) => (
                <ProxyDomainRow key={row.domain} row={row} />
              ))}
            </ul>
          </div>
        ) : null}
        <div className="flex items-center gap-2">
          <Button
            type="button"
            disabled={blocked || pending}
            onClick={() => setConfirming(true)}
          >
            {pending ? (
              <SpinnerIcon className="animate-spin" aria-hidden="true" />
            ) : null}
            {pending ? t('proxySetup.settingUp') : t('proxySetup.setup')}
          </Button>
        </div>
      </CardContent>
      <ProxySetupConfirm
        open={confirming}
        onOpenChange={setConfirming}
        directory={data.detected.dynamic_dir}
        domains={data.domains.map((d) => d.domain)}
        pending={pending}
        onConfirm={onConfirm}
      />
    </Card>
  )
}
