import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { PolicyCheckResults } from '../../components/policy/PolicyCheckResults'
import { PolicyRuleEditor } from '../../components/policy/PolicyRuleEditor'
import {
  inputsFromRules,
  rulesFromInputs,
  type RuleInputs,
} from '../../components/policy/policyRules'
import { useIsRoot } from '../../hooks/useIsRoot'
import { ApiError } from '../../lib/apiError'
import {
  POLICY_RULE_IDS,
  deployPoliciesQueryOptions,
  useCheckDeployPolicy,
  useClearDeployPolicy,
  useSaveDeployPolicy,
  type DeployPolicy,
  type PolicyMode,
  type PolicyRuleId,
  type PolicyScope,
} from '../../queries/deployPolicies'

export const Route = createFileRoute('/settings/deploy-policies')({
  component: DeployPoliciesPage,
})

const SCOPES: PolicyScope[] = ['org', 'project', 'environment']
const ORG_ID = 'org'

function DeployPoliciesPage() {
  const { t } = useTranslation('deployPolicies')
  const isRoot = useIsRoot()
  const { data, isPending, error } = useQuery(deployPoliciesQueryOptions())
  const save = useSaveDeployPolicy()
  const clear = useClearDeployPolicy()
  const check = useCheckDeployPolicy()
  const [scope, setScope] = useState<PolicyScope>('org')
  const [scopeId, setScopeId] = useState('')
  const [modes, setModes] = useState<Partial<Record<PolicyRuleId, PolicyMode>>>(
    {},
  )
  const [inputs, setInputs] = useState<RuleInputs>({})
  const [formError, setFormError] = useState<string | null>(null)
  const notConfigured = error instanceof ApiError && error.status === 501
  const id = scope === 'org' ? ORG_ID : scopeId.trim()

  function load(p: DeployPolicy) {
    setScope(p.scope)
    setScopeId(p.scope === 'org' ? '' : p.scope_id)
    const next: Partial<Record<PolicyRuleId, PolicyMode>> = {}
    for (const rid of POLICY_RULE_IDS) {
      const r = p.rules[rid]
      if (r) next[rid] = r.mode
    }
    setModes(next)
    setInputs(inputsFromRules(p.rules))
    setFormError(null)
  }

  function build() {
    if (id === '') {
      setFormError(t('errors.idRequired'))
      return null
    }
    const built = rulesFromInputs(modes, inputs)
    if ('badRule' in built) {
      setFormError(
        t('errors.numberRequired', { rule: t(`rules.${built.badRule}.title`) }),
      )
      return null
    }
    setFormError(null)
    return built.rules
  }

  function onSave() {
    const rules = build()
    if (!rules) return
    save.mutate(
      { scope, id, rules },
      {
        onSuccess: () => toast.add({ type: 'success', title: t('saved') }),
        onError: (err) => toast.add({ type: 'error', title: err.message }),
      },
    )
  }

  function onClear() {
    if (id === '') {
      setFormError(t('errors.idRequired'))
      return
    }
    clear.mutate(
      { scope, id },
      {
        onSuccess: () => {
          setModes({})
          setInputs({})
          toast.add({ type: 'success', title: t('cleared') })
        },
        onError: (err) => toast.add({ type: 'error', title: err.message }),
      },
    )
  }

  function onDryRun() {
    const rules = build()
    if (!rules) return
    check.mutate({ scope, scope_id: id, rules })
  }

  return (
    <PageTemplate
      icon={<ShieldCheckIcon />}
      title={t('title')}
      description={t('description')}
    >
      {notConfigured ? (
        <Banner variant="critical" title={t('notConfigured')} />
      ) : null}
      {error && !notConfigured ? (
        <Banner variant="critical" title={error.message} />
      ) : null}
      {isRoot ? null : <Banner variant="info" title={t('rootOnly')} />}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="dp-scope">{t('scope.label')}</FieldLabel>
          <Select
            value={scope}
            onValueChange={(v) => {
              setScope(v as PolicyScope)
            }}
          >
            <SelectTrigger id="dp-scope" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SCOPES.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`scope.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {scope === 'org' ? null : (
          <Field data-invalid={formError ? true : undefined}>
            <FieldLabel htmlFor="dp-scope-id">{t('scope.idLabel')}</FieldLabel>
            <Input
              id="dp-scope-id"
              value={scopeId}
              placeholder={t('scope.idPlaceholder')}
              onChange={(e) => {
                setScopeId(e.target.value)
              }}
              aria-invalid={!!formError}
            />
          </Field>
        )}
      </div>
      <PolicyRuleEditor
        modes={modes}
        inputs={inputs}
        disabled={!isRoot}
        onMode={(rid, m) => {
          setModes((prev) => ({ ...prev, [rid]: m }))
        }}
        onInput={(rid, v) => {
          setInputs((prev) => ({ ...prev, [rid]: v }))
        }}
      />
      {formError ? <Banner variant="critical" title={formError} /> : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={!isRoot || save.isPending} onClick={onSave}>
          {t('save')}
        </Button>
        <Button variant="outline" disabled={check.isPending} onClick={onDryRun}>
          {t('dryRun')}
        </Button>
        <Button
          variant="outline"
          disabled={!isRoot || clear.isPending}
          onClick={onClear}
        >
          {t('clear')}
        </Button>
        <span className="text-xs text-muted-foreground">{t('dryRunHint')}</span>
      </div>
      {check.error ? (
        <Banner variant="critical" title={check.error.message} />
      ) : null}
      {check.data ? <PolicyCheckResults result={check.data} /> : null}
      <section className="space-y-2">
        <h2 className="text-sm font-medium">{t('stored.title')}</h2>
        {!isPending && data?.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('stored.empty')}</p>
        ) : null}
        {data?.map((p) => (
          <Card key={`${p.scope}/${p.scope_id}`}>
            <CardContent className="flex flex-wrap items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">
                  {t(`scope.${p.scope}`)}
                  {p.scope === 'org' ? '' : `: ${p.scope_id}`}
                </p>
                <p className="text-xs text-muted-foreground">
                  {Object.keys(p.rules)
                    .map((rid) => t(`rules.${rid as PolicyRuleId}.title`))
                    .join(', ')}
                </p>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  load(p)
                }}
              >
                {t('stored.edit')}
              </Button>
            </CardContent>
          </Card>
        ))}
      </section>
    </PageTemplate>
  )
}
