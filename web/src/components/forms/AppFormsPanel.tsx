import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ClipboardTextIcon, PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { useAppForms } from '../../queries/forms'
import { CreateFormDialog } from './CreateFormDialog'
import { FormCard } from './FormCard'

export function AppFormsPanel({ app }: { app: string }) {
  const { t } = useTranslation('forms')
  const forms = useAppForms(app)
  const [creating, setCreating] = useState(false)

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="max-w-2xl">
          <h2 className="text-lg font-semibold">{t('title')}</h2>
          <p className="text-sm text-muted-foreground">{t('description')}</p>
        </div>
        <Button writeAction onClick={() => setCreating(true)}>
          <PlusIcon />
          {t('new')}
        </Button>
      </header>

      {forms.isPending && (
        <p role="status" className="text-sm text-muted-foreground">
          {t('loading')}
        </p>
      )}
      {forms.isError && (
        <Card>
          <CardHeader>
            <CardTitle>{t('loadError')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Button variant="outline" onClick={() => void forms.refetch()}>
              {t('retry')}
            </Button>
          </CardContent>
        </Card>
      )}
      {forms.isSuccess && forms.data.forms.length === 0 && (
        <EmptyState
          icon={<ClipboardTextIcon className="size-5" />}
          title={t('empty.title')}
          description={t('empty.description')}
          action={
            <Button writeAction onClick={() => setCreating(true)}>
              <PlusIcon />
              {t('new')}
            </Button>
          }
        />
      )}
      {forms.isSuccess &&
        forms.data.forms.map((f) => (
          <FormCard
            key={f.id}
            app={app}
            form={f}
            retentionDays={forms.data.retention_days}
          />
        ))}

      <CreateFormDialog app={app} open={creating} onOpenChange={setCreating} />
    </div>
  )
}
