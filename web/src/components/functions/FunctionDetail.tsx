import { Suspense, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeftGlassIcon } from '@thesvg/ui-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { FunctionEnvironmentTab } from './FunctionEnvironmentTab'
import { FunctionKVTab } from './FunctionKVTab'
import { FunctionOverviewTab } from './FunctionOverviewTab'
import { FunctionSettingsTab } from './FunctionSettingsTab'
import { FunctionVersionsTab } from './FunctionVersionsTab'

export interface FunctionDetailTab {
  id: string
  label: string
  render: (name: string) => ReactNode
}

interface FunctionDetailProps {
  name: string
  extraTabs?: FunctionDetailTab[]
}

/** FunctionDetail is the per-function page; extraTabs append more tabs after the built-in four. */
export function FunctionDetail({ name, extraTabs = [] }: FunctionDetailProps) {
  const { t } = useTranslation('functions')
  const builtIn: FunctionDetailTab[] = [
    {
      id: 'overview',
      label: t('detail.tabs.overview'),
      render: (n) => <FunctionOverviewTab name={n} />,
    },
    {
      id: 'settings',
      label: t('detail.tabs.settings'),
      render: (n) => <FunctionSettingsTab name={n} />,
    },
    {
      id: 'environment',
      label: t('detail.tabs.environment'),
      render: (n) => <FunctionEnvironmentTab name={n} />,
    },
    {
      id: 'versions',
      label: t('detail.tabs.versions'),
      render: (n) => <FunctionVersionsTab name={n} />,
    },
    {
      id: 'kv',
      label: t('detail.tabs.kv'),
      render: (n) => <FunctionKVTab name={n} />,
    },
  ]
  const tabs = [...builtIn, ...extraTabs]

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <Link
          to="/functions"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftGlassIcon aria-hidden className="size-5" />
          {t('detail.back')}
        </Link>
        <h1 className="text-xl font-semibold">{name}</h1>
      </div>
      <Tabs defaultValue="overview">
        <TabsList variant="line">
          {tabs.map((tab) => (
            <TabsTrigger key={tab.id} value={tab.id}>
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>
        {tabs.map((tab) => (
          <TabsContent key={tab.id} value={tab.id} className="pt-4">
            <Suspense
              fallback={
                <div className="h-24 animate-pulse rounded-md bg-muted" />
              }
            >
              {tab.render(name)}
            </Suspense>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
