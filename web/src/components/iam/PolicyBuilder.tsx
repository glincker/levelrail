import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PlusIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import {
  attachPrincipal,
  createPolicy,
  iamPolicyKeys,
  updatePolicy,
} from '../../queries/iamPolicies'
import type { PolicyDocument, PolicyResource } from '../../queries/iamPolicies'
import {
  iamBuilderKeys,
  iamCatalogQueryOptions,
  iamPrincipalsQueryOptions,
  iamResourcesQueryOptions,
  validatePolicy,
} from '../../queries/iamBuilder'
import type { PreviewResult } from '../../queries/iamBuilder'
import {
  documentFromJson,
  documentToStatements,
  draftToDocument,
  emptyDraft,
  emptyStatement,
} from '../../lib/iamDraft'
import type { PolicyDraft } from '../../lib/iamDraft'
import { ImpactPreview } from './ImpactPreview'
import { JsonPanel } from './JsonPanel'
import { StatementEditor } from './StatementEditor'
import { TemplateStart } from './TemplateStart'
import { PrincipalChecklist } from './PrincipalChecklist'

const VALIDATE_DEBOUNCE_MS = 300

function initialDraft(policy: PolicyResource | undefined): PolicyDraft {
  if (!policy) return emptyDraft()
  return {
    name: policy.name,
    description: policy.description,
    statements: documentToStatements(policy.document),
  }
}

/** PolicyBuilder is the guided editor: rules are built from pickers, the stored JSON is generated beside them, and the effect is previewed before save. */
export function PolicyBuilder({
  open,
  onOpenChange,
  policy,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  policy?: PolicyResource
}) {
  const { t } = useTranslation('iam')
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<PolicyDraft>(() => initialDraft(policy))
  const [advanced, setAdvanced] = useState(false)
  const [jsonText, setJsonText] = useState('')
  const [attachTo, setAttachTo] = useState<string[]>([])
  const [preview, setPreview] = useState<PreviewResult | undefined>()
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [nameError, setNameError] = useState(false)

  const catalog = useQuery(iamCatalogQueryOptions())
  const resources = useQuery(iamResourcesQueryOptions())
  const principals = useQuery({
    ...iamPrincipalsQueryOptions(),
    enabled: open && !policy,
  })

  const guidedDoc = useMemo(() => draftToDocument(draft), [draft])
  const parsed = advanced ? documentFromJson(jsonText) : { doc: guidedDoc }
  const doc: PolicyDocument | null = 'doc' in parsed ? parsed.doc : null
  const docKey = useDebouncedValue(JSON.stringify(doc), VALIDATE_DEBOUNCE_MS)

  const validation = useQuery({
    queryKey: [...iamBuilderKeys.all, 'validate', docKey],
    queryFn: () => validatePolicy(JSON.parse(docKey) as PolicyDocument),
    enabled: open && doc !== null,
  })
  const issues = validation.data?.issues ?? []
  const hasError = !doc || issues.some((i) => i.severity === 'error')
  const blocked = preview?.guard.blocked ?? false

  const previewRequest = useMemo(() => {
    if (!doc) return {}
    if (policy) return { policy_id: policy.id, document: doc }
    return {
      document: doc,
      attach: attachTo.map((k) => {
        const [type, ...rest] = k.split(':')
        return {
          principal_type:
            type === 'user' ? ('user' as const) : ('token' as const),
          principal_id: rest.join(':'),
        }
      }),
    }
  }, [doc, policy, attachTo])
  const previewEnabled =
    open &&
    doc !== null &&
    !hasError &&
    (Boolean(policy) || attachTo.length > 0)

  const toggleAdvanced = (on: boolean) => {
    if (on) {
      setJsonText(JSON.stringify(guidedDoc, null, 2))
    } else if (doc) {
      setDraft((d) => ({ ...d, statements: documentToStatements(doc) }))
    }
    setAdvanced(on)
  }

  const save = async () => {
    if (!draft.name.trim()) {
      setNameError(true)
      return
    }
    if (!doc) return
    setSaving(true)
    setSaveError(null)
    try {
      const body = {
        name: draft.name.trim(),
        description: draft.description,
        document: doc,
      }
      const saved = policy
        ? await updatePolicy(policy.id, body)
        : await createPolicy(body)
      for (const k of attachTo) {
        const [type, ...rest] = k.split(':')
        await attachPrincipal({
          policyId: saved.id,
          principal_type: type === 'user' ? 'user' : 'token',
          principal_id: rest.join(':'),
        })
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: iamPolicyKeys.all }),
        queryClient.invalidateQueries({ queryKey: iamBuilderKeys.all }),
      ])
      toast.add({
        title: t('builder.saved', { name: saved.name }),
        type: 'success',
      })
      onOpenChange(false)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : t('errors.generic'))
    } finally {
      setSaving(false)
    }
  }

  const ready = catalog.data && resources.data

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>
            {policy
              ? t('builder.editTitle', { name: policy.name })
              : t('builder.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('builder.description')}</DialogDescription>
        </DialogHeader>

        {!ready ? (
          catalog.isError || resources.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                {(catalog.error ?? resources.error)?.message}
              </AlertDescription>
            </Alert>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('impact.checking')}
            </p>
          )
        ) : (
          <div className="grid gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
            <div className="min-w-0 space-y-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor="iam-policy-name">
                    {t('builder.name')}
                  </FieldLabel>
                  <Input
                    id="iam-policy-name"
                    value={draft.name}
                    placeholder={t('builder.namePlaceholder')}
                    aria-invalid={nameError}
                    onChange={(e) => {
                      setNameError(false)
                      setDraft((d) => ({ ...d, name: e.target.value }))
                    }}
                  />
                  <FieldError
                    errors={[
                      nameError
                        ? { message: t('builder.nameRequired') }
                        : undefined,
                    ]}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="iam-policy-description">
                    {t('builder.descriptionLabel')}
                  </FieldLabel>
                  <Textarea
                    id="iam-policy-description"
                    rows={1}
                    value={draft.description}
                    placeholder={t('builder.descriptionPlaceholder')}
                    onChange={(e) =>
                      setDraft((d) => ({ ...d, description: e.target.value }))
                    }
                  />
                </Field>
              </div>

              {!policy && !advanced ? (
                <TemplateStart
                  resources={resources.data}
                  onApply={(r) =>
                    setDraft((d) => ({
                      name: d.name || r.name,
                      description: d.description || r.description,
                      statements: documentToStatements(r.document),
                    }))
                  }
                />
              ) : null}

              {advanced ? (
                <Alert>
                  <WarningIcon />
                  <AlertDescription>
                    {t('builder.json.advancedHint')}
                  </AlertDescription>
                </Alert>
              ) : (
                <div className="space-y-3">
                  {draft.statements.map((s, i) => (
                    <StatementEditor
                      key={i}
                      index={i}
                      statement={s}
                      abilities={catalog.data.abilities}
                      resources={resources.data}
                      issues={issues}
                      canRemove={draft.statements.length > 1}
                      onChange={(next) =>
                        setDraft((d) => ({
                          ...d,
                          statements: d.statements.map((x, j) =>
                            j === i ? next : x,
                          ),
                        }))
                      }
                      onRemove={() =>
                        setDraft((d) => ({
                          ...d,
                          statements: d.statements.filter((_, j) => j !== i),
                        }))
                      }
                    />
                  ))}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      setDraft((d) => ({
                        ...d,
                        statements: [...d.statements, emptyStatement()],
                      }))
                    }
                  >
                    <PlusIcon />
                    {t('builder.addStatement')}
                  </Button>
                </div>
              )}

              {!policy ? (
                <PrincipalChecklist
                  principals={principals.data ?? []}
                  selected={attachTo}
                  onChange={setAttachTo}
                />
              ) : null}
            </div>

            <div className="min-w-0 space-y-4">
              <JsonPanel
                advanced={advanced}
                onAdvancedChange={toggleAdvanced}
                guidedDocument={guidedDoc}
                jsonText={jsonText}
                onJsonTextChange={setJsonText}
                jsonError={'error' in parsed ? parsed.error : null}
                issues={issues}
                findings={validation.data?.findings ?? []}
                checking={validation.isFetching}
              />
              <ImpactPreview
                request={previewRequest}
                enabled={previewEnabled}
                onResult={setPreview}
              />
            </div>
          </div>
        )}

        {saveError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('builder.cancel')}
          </Button>
          <Button
            type="button"
            onClick={() => void save()}
            disabled={saving || hasError || blocked || !ready}
          >
            {saving ? t('builder.saving') : t('builder.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
