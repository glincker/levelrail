import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { RuleBuilder } from '../rules/RuleBuilder'
import { blankDraft, type RuleDraft, type RuleErrors } from '../rules/ruleModel'
import { RuleSelect } from '../rules/RuleSelect'
import { domainsQueryOptions } from '../../queries/domains'
import {
  FirewallError,
  useCreateFirewallRule,
  useFirewallRules,
  useFirewallStatus,
  useUpdateFirewallRule,
  type FwRule,
} from '../../queries/firewallEdge'
import {
  draftForValue,
  draftFromRule,
  mapServerErrors,
  ruleFromDraft,
  ruleJson,
  validateDraft,
} from './firewallRuleModel'
import { LoadError } from './LoadError'
import type { RuleSeed } from './useFirewallActions'
import { useFirewallCatalog, useFirewallLabels } from './useFirewallLabels'

/** RuleEditor creates or edits one firewall rule with the shared RuleBuilder, validating on both sides. */
export function RuleEditor({
  ruleId,
  seed,
  initialScope,
  onDone,
}: {
  ruleId?: string
  seed?: RuleSeed
  initialScope: string
  onDone: () => void
}) {
  const rules = useFirewallRules()
  const existing = ruleId ? rules.data?.find((r) => r.id === ruleId) : undefined
  if (ruleId && rules.error) {
    return (
      <LoadError error={rules.error} onRetry={() => void rules.refetch()} />
    )
  }
  if (ruleId && !rules.data) {
    return <Skeleton aria-hidden="true" className="h-96 w-full" />
  }
  return (
    <EditorForm
      key={existing?.id ?? 'new'}
      existing={existing}
      missing={Boolean(ruleId) && !existing}
      seed={seed}
      initialScope={initialScope}
      onDone={onDone}
    />
  )
}

function EditorForm({
  existing,
  missing,
  seed,
  initialScope,
  onDone,
}: {
  existing?: FwRule
  missing: boolean
  seed?: RuleSeed
  initialScope: string
  onDone: () => void
}) {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const status = useFirewallStatus('', '1h')
  const domains = useQuery(domainsQueryOptions())
  const { fields, actions } = useFirewallCatalog(
    status.data?.geo_active ?? false,
  )
  const create = useCreateFirewallRule()
  const update = useUpdateFirewallRule()
  const [draft, setDraft] = useState<RuleDraft>(() =>
    existing
      ? draftFromRule(existing)
      : seed
        ? draftForValue(seed.field, seed.value)
        : blankDraft(fields),
  )
  const [scope, setScope] = useState(existing?.scope ?? initialScope)
  const [serverErrors, setServerErrors] = useState<RuleErrors>({})
  const [error, setError] = useState<string | null>(null)
  const [lockout, setLockout] = useState(false)
  const enabled = existing?.enabled ?? true
  const pending = create.isPending || update.isPending

  if (missing) {
    return (
      <p className="text-sm text-muted-foreground">{t('editor.missing')}</p>
    )
  }

  function submit(force = false) {
    setError(null)
    setServerErrors({})
    const rule = ruleFromDraft(draft, scope, enabled)
    const opts = {
      onSuccess: () => {
        toast.add({
          title: existing ? t('editor.updated') : t('editor.created'),
          type: 'success',
        })
        onDone()
      },
      onError: (e: Error) => {
        if (e instanceof FirewallError) {
          if (e.code === 'self_lockout') {
            setLockout(true)
            return
          }
          if (e.fields.length > 0) {
            setServerErrors(mapServerErrors(e.fields))
            return
          }
        }
        setError(e.message)
      },
    }
    if (existing) update.mutate({ id: existing.id, rule, force }, opts)
    else create.mutate({ rule, force }, opts)
  }

  const hosts = [...new Set((domains.data ?? []).map((d) => d.domain))].sort()
  if (scope && !hosts.includes(scope)) hosts.unshift(scope)

  return (
    <>
      <RuleBuilder
        fields={fields}
        actions={actions}
        value={draft}
        onChange={setDraft}
        validate={(d) => validateDraft(d, labels.messages)}
        serverErrors={serverErrors}
        toCode={(d) => ruleJson(d, scope, enabled)}
        onSubmit={() => submit()}
        onCancel={onDone}
        submitLabel={existing ? t('editor.save') : t('editor.create')}
        pendingLabel={t('editor.saving')}
        submitting={pending}
        error={error}
        namePlaceholder={t('editor.namePlaceholder')}
        notePlaceholder={t('editor.notePlaceholder')}
        extra={
          <label className="block max-w-sm space-y-1.5 text-sm font-medium">
            {t('editor.scope')}
            <RuleSelect
              className="w-full"
              value={scope}
              options={[
                { value: '', label: t('page.allHosts') },
                ...hosts.map((h) => ({ value: h, label: h })),
              ]}
              onChange={(e) => setScope(e.target.value)}
            />
            <span className="block text-xs font-normal text-muted-foreground">
              {t('editor.scopeHelp')}
            </span>
          </label>
        }
      />
      <ConfirmDialog
        open={lockout}
        onOpenChange={setLockout}
        title={t('lockout.title')}
        description={t('lockout.ruleDescription')}
        confirmLabel={t('lockout.confirm')}
        tone="destructive"
        pending={pending}
        onConfirm={() => {
          setLockout(false)
          submit(true)
        }}
      />
    </>
  )
}
