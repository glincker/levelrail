import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  LightningIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageTemplate } from '../shell/PageTemplate'
import { useFunctions, type FunctionWindow } from '../../queries/functions'
import { DeployFunctionDialog } from './DeployFunctionDialog'
import { FunctionTemplates } from './FunctionTemplates'
import { FunctionWorkbench } from './FunctionWorkbench'
import { FunctionsTable } from './FunctionsTable'

const WINDOWS: FunctionWindow[] = ['1h', '24h', '7d']

export function FunctionsPage() {
  const { t } = useTranslation('functions')
  const [window, setWindow] = useState<FunctionWindow>('24h')
  const query = useFunctions(window)
  const items = query.data?.functions ?? []

  let body
  if (query.isPending) {
    body = <Skeleton className="h-32 w-full" />
  } else if (query.isError) {
    body = (
      <EmptyState
        icon={<WarningCircleIcon />}
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
  } else if (items.length === 0) {
    body = (
      <EmptyState
        icon={<LightningIcon />}
        title={t('empty.title')}
        description={t('empty.body')}
      />
    )
  } else {
    body = <FunctionsTable items={items} />
  }

  return (
    <PageTemplate
      title={t('title')}
      description={t('description')}
      icon={<LightningIcon />}
      actions={<DeployFunctionDialog />}
    >
      <div className="space-y-8">
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
        <FunctionWorkbench items={items} />
        <FunctionTemplates />
      </div>
    </PageTemplate>
  )
}
