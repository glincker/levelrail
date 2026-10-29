import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { EmptyState } from '@/components/ui/empty-state'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { stackTemplatesQueryOptions } from '../../../queries/stackTemplates'
import { StackTemplateCard } from './StackTemplateCard'
import { StackTemplateDrawer } from './StackTemplateDrawer'

export function StackTemplatesPage() {
  const { t } = useTranslation('stacks')
  const [openId, setOpenId] = useState<string | null>(null)
  const {
    data: templates,
    isPending,
    isError,
  } = useQuery(stackTemplatesQueryOptions())

  return (
    <PageTemplate
      icon={<LayoutGridGlassIcon size="1.2em" />}
      title={t('templates.title')}
      description={t('templates.description')}
    >
      {isPending ? (
        <CardSkeleton />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('templates.loadFailed')}</p>
      ) : templates.length === 0 ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<LayoutGridGlassIcon className="size-5.5" />}
          title={t('templates.empty.title')}
          description={t('templates.empty.description')}
        />
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {templates.map((tpl) => (
            <li key={tpl.id} className="flex">
              <StackTemplateCard
                template={tpl}
                onOpen={() => {
                  setOpenId(tpl.id)
                }}
              />
            </li>
          ))}
        </ul>
      )}
      <StackTemplateDrawer
        id={openId}
        onClose={() => {
          setOpenId(null)
        }}
      />
    </PageTemplate>
  )
}
