import { useMemo, useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SocialGithubGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  MAIN_FIELD,
  TEMPLATE_FIELDS,
  browserInfo,
  buildDraft,
  buildIssueUrl,
  issueUrlMaxChars,
  routeTemplate,
  type IssueSource,
} from '@/lib/issueReport'
import { updatesQueryOptions } from '@/queries/updates'

export interface ReportIssueDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  source: IssueSource
  supportUrl: string
  errorMessage?: string
  requestId?: string
}

function ReportBody({
  onOpenChange,
  source,
  supportUrl,
  errorMessage,
  requestId,
}: Omit<ReportIssueDialogProps, 'open'>) {
  const { t } = useTranslation('common')
  const { pathname, routeId } = useRouterState({
    select: (st) => ({
      pathname: st.location.pathname,
      routeId: st.matches[st.matches.length - 1]?.routeId ?? '',
    }),
  })
  // Optional: the error screen may have no session, so never block on it.
  const updates = useQuery({ ...updatesQueryOptions(), retry: false })
  const draft = useMemo(
    () =>
      buildDraft({
        source,
        pathname,
        route: routeTemplate(routeId || pathname),
        version: updates.data?.current_version,
        browser: browserInfo(navigator),
        errorMessage,
        requestId,
      }),
    [source, pathname, routeId, updates.data, errorMessage, requestId],
  )
  const [title, setTitle] = useState<string | null>(null)
  const [edits, setEdits] = useState<Record<string, string>>({})
  const [off, setOff] = useState<Record<string, boolean>>({})

  const mainId = MAIN_FIELD[draft.template]
  const specs = TEMPLATE_FIELDS[draft.template].filter(
    (f) => f.id === mainId || (draft.fields[f.id] ?? '') !== '',
  )
  const sent: Record<string, string> = {}
  for (const f of specs) {
    const v = edits[f.id] ?? draft.fields[f.id] ?? ''
    if (!off[f.id] && v !== '') sent[f.id] = v
  }
  const max = issueUrlMaxChars()
  const url = buildIssueUrl(
    { ...draft, title: title ?? draft.title, fields: sent },
    supportUrl,
    max,
  )

  return (
    <DialogContent className="sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>{t('reportIssue.title')}</DialogTitle>
        <DialogDescription>
          {t('reportIssue.description', {
            template: t(`reportIssue.templates.${draft.template}`),
          })}
        </DialogDescription>
      </DialogHeader>
      <div className="max-h-[60vh] space-y-3 overflow-y-auto">
        <div className="space-y-1.5">
          <Label htmlFor="report-issue-title">
            {t('reportIssue.issueTitle')}
          </Label>
          <Input
            id="report-issue-title"
            value={title ?? draft.title}
            onChange={(e) => {
              setTitle(e.target.value)
            }}
          />
        </div>
        {specs.map((f) => {
          const id = `report-issue-${f.id}`
          const value = edits[f.id] ?? draft.fields[f.id] ?? ''
          const disabled = off[f.id] === true
          return (
            <div key={f.id} className="space-y-1.5">
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`${id}-on`}
                  checked={!disabled}
                  aria-label={t('reportIssue.include', {
                    field: t(`reportIssue.fields.${f.id}`),
                  })}
                  onCheckedChange={(c) => {
                    setOff((o) => ({ ...o, [f.id]: !c }))
                  }}
                />
                <Label htmlFor={id}>{t(`reportIssue.fields.${f.id}`)}</Label>
              </div>
              {f.kind === 'textarea' ? (
                <Textarea
                  id={id}
                  value={value}
                  disabled={disabled}
                  rows={f.id === mainId ? 4 : 3}
                  onChange={(e) => {
                    setEdits((s) => ({ ...s, [f.id]: e.target.value }))
                  }}
                />
              ) : (
                <Input
                  id={id}
                  value={value}
                  disabled={disabled}
                  readOnly={f.kind === 'select'}
                  onChange={(e) => {
                    setEdits((s) => ({ ...s, [f.id]: e.target.value }))
                  }}
                />
              )}
            </div>
          )
        })}
        <p className="text-xs text-muted-foreground">
          {t('reportIssue.privacy')}
        </p>
      </div>
      <DialogFooter className="items-center">
        <p className="mr-auto text-xs text-muted-foreground" aria-live="polite">
          {t('reportIssue.count', { used: url.length, max })}
        </p>
        <Button
          type="button"
          variant="ghost"
          onClick={() => {
            onOpenChange(false)
          }}
        >
          {t('reportIssue.cancel')}
        </Button>
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className={buttonVariants({ variant: 'default' })}
          onClick={() => {
            onOpenChange(false)
          }}
        >
          <SocialGithubGlassIcon size="1.4em" aria-hidden="true" />
          {t('reportIssue.open')}
        </a>
      </DialogFooter>
    </DialogContent>
  )
}

/** Review step before leaving for GitHub; nothing is stored or sent here. */
export function ReportIssueDialog(props: ReportIssueDialogProps) {
  const { open, onOpenChange, ...rest } = props
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <ReportBody onOpenChange={onOpenChange} {...rest} /> : null}
    </Dialog>
  )
}
