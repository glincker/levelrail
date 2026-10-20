import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useFormActions, type AppForm } from '../../queries/forms'
import { FormInbox } from './FormInbox'
import { FormSettings } from './FormSettings'

function CopyBlock({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('forms')
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      toast.add({ title: t('copied'), type: 'success' })
    } catch {
      toast.add({ title: t('copyFailed'), type: 'error' })
    }
  }
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{label}</span>
        <Button variant="ghost" size="xs" onClick={() => void copy()}>
          <CopyIcon />
          {t('copy')}
        </Button>
      </div>
      <pre className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-3 text-xs">
        <code>{value}</code>
      </pre>
    </div>
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
        <CopyBlock label={t('endpoint')} value={form.endpoint} />
        <CopyBlock label={t('snippet')} value={form.snippet} />
        <FormSettings app={app} form={form} />
        <p className="text-xs text-muted-foreground">
          {t('retention', { days: retentionDays })}
        </p>
        <FormInbox app={app} form={form.name} />
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
