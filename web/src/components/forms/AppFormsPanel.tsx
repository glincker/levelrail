import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ClipboardListGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { BetaBadge } from '@/components/BetaBadge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { useAppForms } from '../../queries/forms'
import { staticSitesQueryOptions } from '../../queries/staticSites'
import { CreateFormDialog } from './CreateFormDialog'
import { FormCard } from './FormCard'
import { FormHero } from './FormHero'

export function AppFormsPanel({ app }: { app: string }) {
  const { t } = useTranslation('forms')
  const sites = useQuery(staticSitesQueryOptions())
  const forms = useAppForms(app)
  const [creating, setCreating] = useState(false)

  if (sites.isSuccess && !sites.data.some((s) => s.name === app)) {
    return (
      <EmptyState
        icon={<ClipboardListGlassIcon className="size-6" />}
        title={t('notStatic.title')}
        description={t('notStatic.description')}
        action={
          <Link
            to="/apps/$name"
            params={{ name: app }}
            className={buttonVariants({ variant: 'outline' })}
          >
            {t('notStatic.back')}
          </Link>
        }
      />
    )
  }

  const hasForms = forms.isSuccess && forms.data.forms.length > 0
  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="max-w-2xl">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            {t('title')}
            <BetaBadge hint={t('betaHint')} />
          </h2>
          <p className="text-sm text-muted-foreground">{t('description')}</p>
        </div>
        {hasForms && (
          <Button writeAction onClick={() => setCreating(true)}>
            <PlusGlassIcon size="1.4em" />
            {t('new')}
          </Button>
        )}
      </header>

      {(forms.isSuccess || forms.isPending) && <FormHero compact={hasForms} />}
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
          icon={<ClipboardListGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
          action={
            <Button writeAction onClick={() => setCreating(true)}>
              <PlusGlassIcon size="1.4em" />
              {t('empty.cta')}
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
