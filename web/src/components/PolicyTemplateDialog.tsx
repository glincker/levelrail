import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  policyTemplatesQueryOptions,
  renderTemplateDocument,
  useApplyPolicyTemplate,
} from '../queries/iamPolicyTemplates'
import type { PolicyTemplate } from '../queries/iamPolicyTemplates'
import type { PrincipalType } from '../queries/iamPolicies'

type AttachChoice = 'none' | PrincipalType

const ATTACH_CHOICES: AttachChoice[] = ['none', 'user', 'token']

function missingParam(
  template: PolicyTemplate,
  params: Record<string, string>,
) {
  return template.params.find((p) => p.required && !params[p.name]?.trim())
}

export function PolicyTemplateDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const [selectedId, setSelectedId] = useState('')
  const [params, setParams] = useState<Record<string, string>>({})
  const [attach, setAttach] = useState<AttachChoice>('none')
  const [attachId, setAttachId] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  const templates = useQuery({
    ...policyTemplatesQueryOptions(),
    enabled: open,
  })
  const apply = useApplyPolicyTemplate()

  const list = templates.data?.templates ?? []
  const selected = list.find((tpl) => tpl.id === selectedId) ?? list[0]

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setSelectedId('')
      setParams({})
      setAttach('none')
      setAttachId('')
      setFormError(null)
      apply.reset()
    }
  }

  function selectTemplate(id: string) {
    setSelectedId(id)
    setParams({})
    setFormError(null)
  }

  function handleApply() {
    if (!selected) return
    const missing = missingParam(selected, params)
    if (missing) {
      setFormError(t('iamTemplates.paramRequired', { name: missing.name }))
      return
    }
    if (attach !== 'none' && !attachId.trim()) {
      setFormError(t('iamTemplates.attachIdRequired'))
      return
    }
    setFormError(null)
    apply.mutate(
      {
        id: selected.id,
        params,
        attach:
          attach === 'none'
            ? undefined
            : { principal_type: attach, principal_id: attachId.trim() },
      },
      {
        onSuccess: (res) => {
          handleOpenChange(false)
          toast.add({
            title: t(
              res.attached
                ? 'iamTemplates.toastCreatedAttached'
                : 'iamTemplates.toastCreated',
              { name: res.policy.name },
            ),
            type: 'success',
          })
        },
      },
    )
  }

  const preview = selected ? renderTemplateDocument(selected, params) : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" />}>
        {t('iamTemplates.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheckIcon className="size-4 text-muted-foreground" />
            {t('iamTemplates.title')}
          </DialogTitle>
          <DialogDescription>{t('iamTemplates.description')}</DialogDescription>
        </DialogHeader>

        {templates.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t('iamTemplates.loading')}
          </p>
        ) : null}
        {templates.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{t('iamTemplates.loadError')}</AlertDescription>
          </Alert>
        ) : null}

        {selected && preview ? (
          <div className="space-y-4">
            <div
              role="radiogroup"
              aria-label={t('iamTemplates.templateLabel')}
              className="grid gap-2 sm:grid-cols-2"
            >
              {list.map((tpl) => (
                <button
                  key={tpl.id}
                  type="button"
                  role="radio"
                  aria-checked={tpl.id === selected.id}
                  onClick={() => selectTemplate(tpl.id)}
                  className={cn(
                    'rounded-lg border border-border p-3 text-left text-sm transition-colors hover:bg-muted',
                    tpl.id === selected.id && 'border-primary bg-muted',
                  )}
                >
                  <span className="block font-medium text-foreground">
                    {tpl.name}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {tpl.description}
                  </span>
                </button>
              ))}
            </div>

            {selected.params.map((p) => (
              <Field key={p.name}>
                <FieldLabel htmlFor={`template-param-${p.name}`}>
                  {t('iamTemplates.paramLabel', { name: p.name })}
                </FieldLabel>
                <Input
                  id={`template-param-${p.name}`}
                  placeholder={t('iamTemplates.paramPlaceholder')}
                  value={params[p.name] ?? ''}
                  onChange={(e) =>
                    setParams({ ...params, [p.name]: e.target.value })
                  }
                />
              </Field>
            ))}

            <div className="space-y-1">
              <h3 className="text-sm font-medium text-foreground">
                {t('iamTemplates.effectTitle')}
              </h3>
              <ul className="list-disc space-y-0.5 pl-5 text-sm text-muted-foreground">
                {preview.Statement.map((s, i) => (
                  <li key={i}>
                    {t('iamTemplates.effectLine', {
                      effect: t(
                        s.Effect === 'Allow'
                          ? 'iamTemplates.allow'
                          : 'iamTemplates.deny',
                      ),
                      actions: s.Action.join(', '),
                      resources: s.Resource.join(', '),
                    })}
                  </li>
                ))}
              </ul>
            </div>

            <div className="space-y-1">
              <h3 className="text-sm font-medium text-foreground">
                {t('iamTemplates.previewTitle')}
              </h3>
              <pre className="max-h-48 overflow-auto rounded-lg bg-muted p-3 font-mono text-xs text-foreground">
                {JSON.stringify(preview, null, 2)}
              </pre>
            </div>

            <div className="space-y-2">
              <h3 className="text-sm font-medium text-foreground">
                {t('iamTemplates.attachTitle')}
              </h3>
              <div
                role="radiogroup"
                aria-label={t('iamTemplates.attachTitle')}
                className="flex flex-wrap gap-2"
              >
                {ATTACH_CHOICES.map((choice) => (
                  <Button
                    key={choice}
                    type="button"
                    role="radio"
                    aria-checked={attach === choice}
                    variant={attach === choice ? 'secondary' : 'outline'}
                    onClick={() => setAttach(choice)}
                  >
                    {t(
                      choice === 'none'
                        ? 'iamTemplates.attachNone'
                        : choice === 'user'
                          ? 'iamTemplates.attachUser'
                          : 'iamTemplates.attachToken',
                    )}
                  </Button>
                ))}
              </div>
              {attach !== 'none' ? (
                <Field>
                  <FieldLabel htmlFor="template-attach-id">
                    {t('iamTemplates.attachIdLabel')}
                  </FieldLabel>
                  <Input
                    id="template-attach-id"
                    value={attachId}
                    onChange={(e) => setAttachId(e.target.value)}
                  />
                </Field>
              ) : null}
            </div>
          </div>
        ) : null}

        {formError ? (
          <p role="alert" className="text-sm text-destructive">
            {formError}
          </p>
        ) : null}
        {apply.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{apply.error.message}</AlertDescription>
          </Alert>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            disabled={!selected || apply.isPending}
            onClick={handleApply}
          >
            {apply.isPending
              ? t('iamTemplates.applying')
              : t('iamTemplates.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
