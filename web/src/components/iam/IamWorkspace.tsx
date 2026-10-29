import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { policyListQueryOptions } from '../../queries/iamPolicies'
import type { PolicyResource } from '../../queries/iamPolicies'
import { iamPrincipalsQueryOptions } from '../../queries/iamBuilder'
import { AccessPanel } from './AccessPanel'
import { AnalyzerPanel } from './AnalyzerPanel'
import { AttachDialog } from './AttachDialog'
import { PolicyBuilder } from './PolicyBuilder'
import { PolicyDetail } from './PolicyDetail'
import { PolicyList } from './PolicyList'
import { SimulatorPanel } from './SimulatorPanel'

type Tab = 'policies' | 'access' | 'simulator' | 'analyzer'

/** IamWorkspace is the whole access policy page: policies, who has access, simulator and analyzer, plus the builder and attach flows they open. */
export function IamWorkspace() {
  const { t } = useTranslation('iam')
  const policies = useQuery(policyListQueryOptions())
  const principals = useQuery(iamPrincipalsQueryOptions())
  const [tab, setTab] = useState<Tab>('policies')
  const [builder, setBuilder] = useState<{
    open: boolean
    policy?: PolicyResource
  }>({ open: false })
  const [detailId, setDetailId] = useState<string | undefined>()
  const [attach, setAttach] = useState<{
    mode: 'attach' | 'detach'
    policyId: string
    keys: string[]
  } | null>(null)
  const [focusKey, setFocusKey] = useState<string | undefined>()

  const list = policies.data ?? []
  const detail = list.find((p) => p.id === detailId)

  const openPolicy = (id: string) => {
    setTab('policies')
    setDetailId(id)
  }
  const openPrincipal = (key: string) => {
    setTab('access')
    setFocusKey(key)
  }

  return (
    <PageTemplate
      icon={<ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />}
      title={t('page.title')}
      description={t('page.description')}
      actions={
        <Button onClick={() => setBuilder({ open: true })}>
          <PlusGlassIcon size="1.4em" />
          {t('page.createPolicy')}
        </Button>
      }
    >
      <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)}>
        <TabsList>
          <TabsTrigger value="policies">{t('page.tabs.policies')}</TabsTrigger>
          <TabsTrigger value="access">{t('page.tabs.access')}</TabsTrigger>
          <TabsTrigger value="simulator">
            {t('page.tabs.simulator')}
          </TabsTrigger>
          <TabsTrigger value="analyzer">{t('page.tabs.analyzer')}</TabsTrigger>
        </TabsList>

        <TabsContent value="policies" className="pt-4">
          {policies.isPending ? (
            <Skeleton className="h-48 w-full" />
          ) : policies.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('page.loadError')}: {policies.error.message}{' '}
                <Button
                  variant="link"
                  size="sm"
                  onClick={() => void policies.refetch()}
                >
                  {t('page.retry')}
                </Button>
              </AlertDescription>
            </Alert>
          ) : (
            <PolicyList
              policies={list}
              principals={principals.data ?? []}
              onOpen={(p) => setDetailId(p.id)}
              onCreate={() => setBuilder({ open: true })}
            />
          )}
        </TabsContent>
        <TabsContent value="access" className="pt-4">
          <AccessPanel key={focusKey ?? 'access'} initialKey={focusKey} />
        </TabsContent>
        <TabsContent value="simulator" className="pt-4">
          <SimulatorPanel />
        </TabsContent>
        <TabsContent value="analyzer" className="pt-4">
          <AnalyzerPanel
            onOpenPolicy={openPolicy}
            onOpenPrincipal={openPrincipal}
          />
        </TabsContent>
      </Tabs>

      <PolicyDetail
        policy={detail}
        onClose={() => setDetailId(undefined)}
        onEdit={(p) => setBuilder({ open: true, policy: p })}
        onAttach={(p) =>
          setAttach({ mode: 'attach', policyId: p.id, keys: [] })
        }
        onDetach={(p, key) =>
          setAttach({ mode: 'detach', policyId: p.id, keys: [key] })
        }
      />
      {builder.open ? (
        <PolicyBuilder
          key={builder.policy?.id ?? 'new'}
          open
          onOpenChange={(o) => {
            if (!o) setBuilder({ open: false })
          }}
          policy={builder.policy}
        />
      ) : null}
      {attach ? (
        <AttachDialog
          open
          onOpenChange={(o) => {
            if (!o) setAttach(null)
          }}
          mode={attach.mode}
          policies={list}
          principals={principals.data ?? []}
          policyId={attach.policyId}
          preselected={attach.keys}
        />
      ) : null}
    </PageTemplate>
  )
}
