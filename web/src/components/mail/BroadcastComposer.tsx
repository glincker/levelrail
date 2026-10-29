import { useMemo, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SendGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  planMailBroadcast,
  useMailGroups,
  useMailTemplates,
  useSendMailBroadcast,
  useSendMailBroadcastTest,
  MailApiError,
  type MailBroadcastInput,
  type MailBroadcastPlan,
} from '../../queries/mail'
import { RuleSelect } from '../rules/RuleSelect'
import { TemplatePreview } from './TemplatePreview'
import { TokenPanel } from './TokenPanel'

const DEFAULT_TEMPLATE = 'announcement'

const toLines = (s: string): string[] =>
  s
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')

/** BroadcastComposer writes a message to an audience: template, tokens, preview, test, schedule and a confirmed dry run. */
export function BroadcastComposer({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('mail')
  const { data: templates } = useMailTemplates()
  const { data: groups = [] } = useMailGroups()
  const send = useSendMailBroadcast()
  const test = useSendMailBroadcastTest()
  const announcements = useMemo(
    () =>
      (templates?.templates ?? []).filter((x) => x.category === 'announcement'),
    [templates],
  )
  const [templateKey, setTemplateKey] = useState(DEFAULT_TEMPLATE)
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [audience, setAudience] = useState('')
  const [extra, setExtra] = useState('')
  const [later, setLater] = useState(false)
  const [scheduleAt, setScheduleAt] = useState('')
  const [plan, setPlan] = useState<MailBroadcastPlan | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [planning, setPlanning] = useState(false)
  const bodyRef = useRef<HTMLTextAreaElement>(null)

  const chosen = announcements.find((x) => x.key === templateKey)
  const subjectValue = subject || chosen?.current.subject || ''
  const bodyValue = body || chosen?.current.body || ''

  const input = (): MailBroadcastInput => ({
    subject: subjectValue,
    body: bodyValue,
    template_key: templateKey,
    audience: audience || undefined,
    recipients: toLines(extra),
    schedule_at:
      later && scheduleAt ? new Date(scheduleAt).toISOString() : undefined,
  })

  const draft = useMemo(
    () => ({
      subject: subjectValue,
      preheader: chosen?.current.preheader ?? '',
      body: bodyValue,
    }),
    [subjectValue, bodyValue, chosen],
  )

  const review = async () => {
    setError(null)
    setPlanning(true)
    try {
      setPlan(await planMailBroadcast(input()))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setPlanning(false)
    }
  }

  const confirm = () => {
    if (!plan) return
    send.mutate(
      { ...input(), expected_recipients: plan.deliverable },
      {
        onSuccess: () => {
          toast.add({ title: t('messages.sent'), type: 'success' })
          setPlan(null)
          onOpenChange(false)
        },
        onError: (e) => {
          setPlan(null)
          setError(e.message)
        },
      },
    )
  }

  const onTest = () => {
    test.mutate(
      { subject: subjectValue, body: bodyValue },
      {
        onSuccess: (res) =>
          toast.add({
            title: t('templates.editor.testSent', { email: res.sent_to }),
            type: 'success',
          }),
      },
    )
  }

  const insert = (placeholder: string) => {
    const next = insertAtCursor(bodyRef.current, bodyValue, placeholder)
    setBody(next.value)
  }

  const providerMissing =
    plan?.provider_configured === false ||
    (test.error instanceof MailApiError &&
      test.error.code === 'email_not_configured') ||
    (send.error instanceof MailApiError &&
      send.error.code === 'email_not_configured')

  return (
    <>
      <Dialog open={open && plan === null} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-5xl">
          <DialogHeader>
            <DialogTitle>{t('messages.composer.title')}</DialogTitle>
            <DialogDescription>{t('messages.description')}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-6 lg:grid-cols-2">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="mail-msg-template">
                  {t('messages.composer.template')}
                </FieldLabel>
                <RuleSelect
                  id="mail-msg-template"
                  value={templateKey}
                  options={announcements.map((x) => ({
                    value: x.key,
                    label: x.label,
                  }))}
                  onChange={(e) => {
                    setTemplateKey(e.target.value)
                    setSubject('')
                    setBody('')
                  }}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="mail-msg-subject">
                  {t('messages.composer.subject')}
                </FieldLabel>
                <Input
                  id="mail-msg-subject"
                  value={subjectValue}
                  onChange={(e) => setSubject(e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="mail-msg-body">
                  {t('messages.composer.body')}
                </FieldLabel>
                <Textarea
                  id="mail-msg-body"
                  ref={bodyRef}
                  rows={10}
                  className="font-mono"
                  value={bodyValue}
                  onChange={(e) => setBody(e.target.value)}
                />
                <FieldDescription>
                  {t('messages.composer.bodyHelp', {
                    example: '{{first_name}}',
                  })}
                </FieldDescription>
              </Field>
              <TokenPanel allowed={chosen?.tokens ?? []} onInsert={insert} />
              <Field>
                <FieldLabel htmlFor="mail-msg-audience">
                  {t('messages.composer.audience')}
                </FieldLabel>
                <RuleSelect
                  id="mail-msg-audience"
                  value={audience}
                  options={[
                    { value: '', label: t('messages.composer.noAudience') },
                    ...groups.map((g) => ({
                      value: g.id,
                      label: `${g.name} (${g.recipients})`,
                    })),
                  ]}
                  onChange={(e) => setAudience(e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="mail-msg-extra">
                  {t('messages.composer.extra')}
                </FieldLabel>
                <Textarea
                  id="mail-msg-extra"
                  rows={2}
                  value={extra}
                  onChange={(e) => setExtra(e.target.value)}
                />
                <FieldDescription>
                  {t('messages.composer.extraHelp')}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="mail-msg-when">
                  {t('messages.composer.when')}
                </FieldLabel>
                <RuleSelect
                  id="mail-msg-when"
                  value={later ? 'later' : 'now'}
                  options={[
                    { value: 'now', label: t('messages.composer.now') },
                    { value: 'later', label: t('messages.composer.later') },
                  ]}
                  onChange={(e) => setLater(e.target.value === 'later')}
                />
                {later ? (
                  <Input
                    type="datetime-local"
                    aria-label={t('messages.composer.later')}
                    value={scheduleAt}
                    onChange={(e) => setScheduleAt(e.target.value)}
                  />
                ) : null}
              </Field>
            </FieldGroup>
            <TemplatePreview eventKey={templateKey} draft={draft} />
          </div>
          {providerMissing ? (
            <Alert>
              <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
                <span>{t('messages.noProvider.body')}</span>
                <Link
                  to="/settings/email"
                  className={buttonVariants({ size: 'sm' })}
                >
                  {t('messages.noProvider.fix')}
                </Link>
              </AlertDescription>
            </Alert>
          ) : null}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={test.isPending || subjectValue === ''}
              writeAction
              onClick={onTest}
            >
              {t('messages.composer.sendTest')}
            </Button>
            <Button
              type="button"
              disabled={
                planning ||
                subjectValue === '' ||
                (audience === '' && toLines(extra).length === 0) ||
                (later && scheduleAt === '')
              }
              writeAction
              onClick={() => void review()}
            >
              <SendGlassIcon size="1.2em" data-icon="inline-start" />
              {t('messages.composer.review')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={plan !== null}
        onOpenChange={(o) => {
          if (!o) setPlan(null)
        }}
        title={t('messages.confirm.title')}
        description={
          plan
            ? plan.deliverable === 0
              ? t('messages.confirm.none')
              : t('messages.confirm.summary', {
                  deliverable: plan.deliverable,
                  suppressed: plan.suppressed,
                })
            : undefined
        }
        consequences={
          plan
            ? [
                t('messages.confirm.capLeft', { count: plan.cap_remaining }),
                ...(later && scheduleAt
                  ? [
                      t('messages.confirm.schedule', {
                        time: new Date(scheduleAt).toLocaleString(),
                      }),
                    ]
                  : []),
              ]
            : undefined
        }
        confirmLabel={
          later
            ? t('messages.confirm.schedule_action')
            : t('messages.confirm.send')
        }
        pending={send.isPending}
        error={
          plan && !plan.provider_configured
            ? t('messages.noProvider.body')
            : null
        }
        onConfirm={() => {
          if (plan && plan.deliverable > 0 && plan.provider_configured)
            confirm()
        }}
      >
        {plan && !plan.provider_configured ? (
          <Link to="/settings/email" className={buttonVariants({ size: 'sm' })}>
            {t('messages.noProvider.fix')}
          </Link>
        ) : null}
      </ConfirmDialog>
    </>
  )
}
