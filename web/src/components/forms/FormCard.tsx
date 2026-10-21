import { useCallback, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PaperPlaneTiltIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useFormActions, type AppForm } from '../../queries/forms'
import type { FormSchema } from '../../lib/formSnippet'
import { FormDesigner } from './FormDesigner'
import { FormEmbed } from './FormEmbed'
import { FormInbox } from './FormInbox'
import { FormSettings } from './FormSettings'

function Section({
  title,
  help,
  action,
  children,
}: {
  title: string
  help: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="space-y-3 border-t border-border pt-4">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">{title}</h3>
          <p className="text-xs text-muted-foreground">{help}</p>
        </div>
        {action}
      </header>
      {children}
    </section>
  )
}

function TestButton({ app, form }: { app: string; form: string }) {
  const { t } = useTranslation('forms')
  const { sendTest } = useFormActions(app)
  const send = () =>
    sendTest.mutate(form, {
      onSuccess: (r) =>
        toast.add({
          title: t('test.sent', { id: r.submission_id }),
          description: r.spam ? t('test.flagged') : t('test.ok'),
          type: r.spam ? 'warning' : 'success',
        }),
      onError: (e) =>
        toast.add({
          title: t('test.failed'),
          description: e.message,
          type: 'error',
        }),
    })
  return (
    <Button
      writeAction
      variant="outline"
      size="sm"
      disabled={sendTest.isPending}
      onClick={send}
    >
      <PaperPlaneTiltIcon />
      {sendTest.isPending ? t('test.sending') : t('test.button')}
    </Button>
  )
}

export function FormCard({
  app,
  form,
  retentionDays,
}: {
  app: string
  form: AppForm
  retentionDays: number
}) {
  const { t } = useTranslation('forms')
  const { remove } = useFormActions(app)
  const [confirming, setConfirming] = useState(false)
  const [draft, setDraft] = useState<FormSchema | undefined>()
  const onDraft = useCallback((s: FormSchema) => setDraft(s), [])

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3">
        <div className="min-w-0">
          <CardTitle className="truncate">{form.name}</CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('counts', {
              total: form.submissions,
              spam: form.spam_submissions,
            })}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {form.spam_submissions > 0 && (
            <Badge variant="warning">{form.spam_submissions}</Badge>
          )}
          <Button
            writeAction
            variant="destructive"
            size="sm"
            onClick={() => setConfirming(true)}
          >
            <TrashIcon />
            {t('delete.button')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        <Section
          title={t('sections.design.title')}
          help={t('sections.design.help')}
        >
          <FormDesigner app={app} form={form} onDraft={onDraft} />
        </Section>
        <Section
          title={t('sections.embed.title')}
          help={t('sections.embed.help')}
        >
          <FormEmbed form={form} draft={draft} />
        </Section>
        <Section
          title={t('sections.submissions.title')}
          help={t('sections.submissions.help')}
          action={<TestButton app={app} form={form.name} />}
        >
          <FormInbox app={app} form={form.name} />
          <p className="text-xs text-muted-foreground">
            {t('retention', { days: retentionDays })}
          </p>
        </Section>
        <Section
          title={t('sections.settings.title')}
          help={t('sections.settings.help')}
        >
          <FormSettings app={app} form={form} />
        </Section>
      </CardContent>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('delete.title')}
        description={t('delete.description')}
        consequences={[t('delete.consequence')]}
        confirmLabel={t('delete.confirm')}
        pendingLabel={t('delete.pending')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() =>
          remove.mutate(form.name, { onSuccess: () => setConfirming(false) })
        }
      />
    </Card>
  )
}
