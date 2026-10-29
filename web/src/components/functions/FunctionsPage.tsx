import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, ZapGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageBanner } from '../PageBanner'
import { FunctionsDiagram } from '../banners/FunctionsDiagram'
import { useFunctions, type FunctionWindow } from '../../queries/functions'
import { DeployFunctionDialog } from './DeployFunctionDialog'
import { FunctionTemplates } from './FunctionTemplates'
import { FunctionWorkbench } from './FunctionWorkbench'
import { FunctionTriggers } from './FunctionTriggers'
import { FunctionsTable } from './FunctionsTable'

const WINDOWS: FunctionWindow[] = ['1h', '24h', '7d']

export function FunctionsPage() {
  const { t } = useTranslation('functions')
  const [window, setWindow] = useState<FunctionWindow>('24h')
  const query = useFunctions(window)
  const items = query.data?.functions ?? []
  const isEmpty = query.isSuccess && items.length === 0
  const size = isEmpty ? 'hero' : 'compact'

  let body = null
  if (query.isPending) {
    body = <Skeleton className="h-32 w-full" />
  } else if (query.isError) {
    body = (
      <EmptyState
        icon={
          <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
        }
        title={t('list.error', { message: query.error.message })}
        description=""
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void query.refetch()
            }}
          >
            {t('list.retry')}
          </Button>
        }
      />
    )
  } else if (!isEmpty) {
    body = <FunctionsTable items={items} />
  }

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<ZapGlassIcon size="1.4em" />}
        titleKey="functions:banner.title"
        descriptionKey={
          isEmpty ? 'functions:banner.description' : 'functions:description'
        }
        action={<DeployFunctionDialog />}
        illustration={<FunctionsDiagram size={size} />}
        size={size}
      />
      <div className="space-y-8">
        {isEmpty ? null : (
          <div className="space-y-3">
            <Tabs
              value={window}
              onValueChange={(v) => {
                setWindow(v as FunctionWindow)
              }}
            >
              <TabsList aria-label={t('window.label')}>
                {WINDOWS.map((w) => (
                  <TabsTrigger key={w} value={w}>
                    {t(`window.${w}`)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            {body}
          </div>
        )}
        <FunctionWorkbench items={items} />
        <FunctionTriggers items={items} />
        <FunctionTemplates />
      </div>
    </div>
  )
}
