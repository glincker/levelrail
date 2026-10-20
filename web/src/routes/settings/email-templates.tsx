import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { MailPageShell } from '../../components/mail/MailPageShell'
import { TemplateEditor } from '../../components/mail/TemplateEditor'
import { TemplateList } from '../../components/mail/TemplateList'
import { useMailTemplates } from '../../queries/mail'

export const Route = createFileRoute('/settings/email-templates')({
  component: EmailTemplatesPage,
})

function EmailTemplatesPage() {
  const { t } = useTranslation('mail')
  const { data, isLoading, error, refetch } = useMailTemplates()
  const [editing, setEditing] = useState<string | null>(null)
  const template = data?.templates.find((tpl) => tpl.key === editing)

  return (
    <MailPageShell section="templates">
      {isLoading ? <TableSkeleton columnCount={3} rowCount={6} /> : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>{error.message || t('common.loadError')}</span>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void refetch()}
            >
              {t('common.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {data && template ? (
        <TemplateEditor
          key={template.key}
          template={template}
          onBack={() => setEditing(null)}
        />
      ) : null}
      {data && !template ? (
        <TemplateList
          templates={data.templates}
          layout={data.layout}
          onEdit={setEditing}
        />
      ) : null}
    </MailPageShell>
  )
}
