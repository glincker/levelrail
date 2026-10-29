import { useMemo, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  RotateCcwGlassIcon,
  SendGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { insertAtCursor } from '@/lib/insertAtCursor'
import {
  MailApiError,
  useResetMailTemplate,
  useSaveMailTemplate,
  useSendMailTest,
  type MailTemplate,
} from '../../queries/mail'
import { TemplateHistory } from './TemplateHistory'
import { TemplatePreview } from './TemplatePreview'
import { TokenPanel } from './TokenPanel'

type Target = 'subject' | 'preheader' | 'body'

/** TemplateEditor edits one email template with a token panel, live preview, test send and version history. */
export function TemplateEditor({
  template,
  onBack,
}: {
  template: MailTemplate
  onBack: () => void
}) {
  const { t } = useTranslation('mail')
  const [subject, setSubject] = useState(template.current.subject)
  const [preheader, setPreheader] = useState(template.current.preheader)
  const [body, setBody] = useState(template.current.body)
  const [target, setTarget] = useState<Target>('body')
  const [confirmReset, setConfirmReset] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const subjectRef = useRef<HTMLInputElement>(null)
  const preheaderRef = useRef<HTMLInputElement>(null)
  const bodyRef = useRef<HTMLTextAreaElement>(null)

  const save = useSaveMailTemplate(template.key)
  const reset = useResetMailTemplate(template.key)
  const test = useSendMailTest(template.key)

  const dirty =
    subject !== template.current.subject ||
    preheader !== template.current.preheader ||
    body !== template.current.body
  const draft = useMemo(
    () => ({ subject, preheader, body }),
    [subject, preheader, body],
  )

  const insert = (placeholder: string) => {
    const el =
      target === 'subject'
        ? subjectRef.current
        : target === 'preheader'
          ? preheaderRef.current
          : bodyRef.current
    const current =
      target === 'subject' ? subject : target === 'preheader' ? preheader : body
    const next = insertAtCursor(el, current, placeholder)
    if (target === 'subject') setSubject(next.value)
    else if (target === 'preheader') setPreheader(next.value)
    else setBody(next.value)
    requestAnimationFrame(() => {
      el?.focus()
      el?.setSelectionRange(next.caret, next.caret)
    })
  }

  const onSave = () => {
    setFormError(null)
    save.mutate(draft, {
      onSuccess: () =>
        toast.add({ title: t('templates.editor.saved'), type: 'success' }),
      onError: (error) => setFormError(error.message),
    })
  }

  const onReset = () => {
    reset.mutate(undefined, {
      onSuccess: (fresh) => {
        setSubject(fresh.current.subject)
        setPreheader(fresh.current.preheader)
        setBody(fresh.current.body)
        setConfirmReset(false)
        toast.add({ title: t('templates.editor.resetDone'), type: 'success' })
      },
      onError: (error) => setFormError(error.message),
    })
  }

  const onTest = () => {
    test.mutate(undefined, {
      onSuccess: (res) =>
        toast.add({
          title: t('templates.editor.testSent', { email: res.sent_to }),
          type: 'success',
        }),
    })
  }

  const noProvider =
    test.error instanceof MailApiError &&
    test.error.code === 'email_not_configured'

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeftGlassIcon size="1.2em" data-icon="inline-start" />
          {t('templates.editor.back')}
        </Button>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onTest}
            disabled={test.isPending || dirty}
            writeAction
          >
            <SendGlassIcon size="1.2em" data-icon="inline-start" />
            {t('templates.editor.sendTest')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!template.customized}
            onClick={() => setConfirmReset(true)}
            writeAction
          >
            <RotateCcwGlassIcon size="1.2em" data-icon="inline-start" />
            {t('templates.editor.reset')}
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={onSave}
            disabled={!dirty || save.isPending}
            writeAction
          >
            {save.isPending ? t('common.saving') : t('templates.editor.save')}
          </Button>
        </div>
      </div>

      {noProvider ? (
        <Alert>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>
              <strong>{t('templates.editor.noProviderTitle')}</strong>{' '}
              {t('templates.editor.noProviderBody')}
            </span>
            <Link
              to="/settings/email"
              className={buttonVariants({ size: 'sm' })}
            >
              {t('templates.editor.fixProvider')}
            </Link>
          </AlertDescription>
        </Alert>
      ) : test.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{test.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {formError ? (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="space-y-6">
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="mail-subject">
                {t('templates.editor.subject')}
              </FieldLabel>
              <Input
                id="mail-subject"
                ref={subjectRef}
                value={subject}
                onFocus={() => setTarget('subject')}
                onChange={(e) => setSubject(e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="mail-preheader">
                {t('templates.editor.preheader')}
              </FieldLabel>
              <Input
                id="mail-preheader"
                ref={preheaderRef}
                value={preheader}
                onFocus={() => setTarget('preheader')}
                onChange={(e) => setPreheader(e.target.value)}
              />
              <FieldDescription>
                {t('templates.editor.preheaderHelp')}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="mail-body">
                {t('templates.editor.body')}
              </FieldLabel>
              <Textarea
                id="mail-body"
                ref={bodyRef}
                value={body}
                rows={14}
                className="font-mono"
                onFocus={() => setTarget('body')}
                onChange={(e) => setBody(e.target.value)}
              />
              <FieldDescription>
                {t('templates.editor.bodyHelp')}
              </FieldDescription>
            </Field>
          </FieldGroup>
          <TokenPanel allowed={template.tokens} onInsert={insert} />
          <TemplateHistory
            eventKey={template.key}
            onRestored={(v) => {
              setSubject(v.subject)
              setPreheader(v.preheader)
              setBody(v.body)
            }}
          />
        </div>
        <TemplatePreview eventKey={template.key} draft={draft} />
      </div>

      <ConfirmDialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={t('templates.editor.resetTitle')}
        description={t('templates.editor.resetDescription')}
        confirmLabel={t('templates.editor.reset')}
        pending={reset.isPending}
        onConfirm={onReset}
      />
    </div>
  )
}
