import { type FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  type ApprovalScope,
  type SecurityPolicy,
  accountSecurityQueryOptions,
  securityPolicyQueryOptions,
  useUpdateAccountSecurity,
  useUpdateSecurityPolicy,
} from '../../queries/securityCenter'

type DayField =
  'max_token_lifetime_days' | 'warn_unused_days' | 'disable_unused_days'

const DAY_FIELDS: {
  key: DayField
  label: 'policy.maxLifetime' | 'policy.warnUnused' | 'policy.disableUnused'
}[] = [
  { key: 'max_token_lifetime_days', label: 'policy.maxLifetime' },
  { key: 'warn_unused_days', label: 'policy.warnUnused' },
  { key: 'disable_unused_days', label: 'policy.disableUnused' },
]

function sourceKey(source: string | undefined) {
  if (source === 'saved') return 'policy.source.saved' as const
  if (source === 'env') return 'policy.source.env' as const
  return 'policy.source.default' as const
}

function PolicyForm({
  policy,
  editable,
}: {
  policy: SecurityPolicy
  editable: boolean
}) {
  const { t } = useTranslation('security')
  const update = useUpdateSecurityPolicy()
  const [scope, setScope] = useState<ApprovalScope>(policy.approval_scope)
  const [days, setDays] = useState<Record<DayField, string>>({
    max_token_lifetime_days: String(policy.max_token_lifetime_days),
    warn_unused_days: String(policy.warn_unused_days),
    disable_unused_days: String(policy.disable_unused_days),
  })
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const body: Partial<Record<DayField, number>> & {
      approval_scope: ApprovalScope
    } = {
      approval_scope: scope,
    }
    for (const f of DAY_FIELDS) {
      const n = Number.parseInt(days[f.key], 10)
      if (Number.isFinite(n) && n >= 0) body[f.key] = n
    }
    update.mutate(body, {
      onSuccess: () => toast.add({ title: t('policy.saved'), type: 'success' }),
      onError: (err) =>
        toast.add({
          title: err.message || t('policy.saveError'),
          type: 'error',
        }),
    })
  }
  return (
    <form className="space-y-4" onSubmit={onSubmit}>
      {policy.new_device_approval ? null : (
        <Alert>
          <WarningIcon />
          <AlertDescription>{t('policy.approvalOff')}</AlertDescription>
        </Alert>
      )}
      <Field>
        <FieldLabel htmlFor="approval-scope">
          {t('policy.approvalScope')}
        </FieldLabel>
        <Select
          value={scope}
          disabled={!editable}
          onValueChange={(v) =>
            setScope(v === 'all_methods' ? 'all_methods' : 'password_only')
          }
        >
          <SelectTrigger id="approval-scope" className="w-full max-w-md">
            <SelectValue>
              {(v: string) =>
                v === 'all_methods'
                  ? t('policy.scopeAllMethods')
                  : t('policy.scopePasswordOnly')
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="password_only">
              {t('policy.scopePasswordOnly')}
            </SelectItem>
            <SelectItem value="all_methods">
              {t('policy.scopeAllMethods')}
            </SelectItem>
          </SelectContent>
        </Select>
        <FieldDescription>
          {t(sourceKey(policy.sources.approval_scope))}
        </FieldDescription>
      </Field>
      {DAY_FIELDS.map((f) => (
        <Field key={f.key}>
          <FieldLabel htmlFor={f.key}>{t(f.label)}</FieldLabel>
          <Input
            id={f.key}
            type="number"
            min={0}
            className="max-w-40"
            disabled={!editable}
            value={days[f.key]}
            onChange={(e) =>
              setDays((d) => ({ ...d, [f.key]: e.target.value }))
            }
          />
          <FieldDescription>
            {t(sourceKey(policy.sources[f.key]))}
            {f.key === 'disable_unused_days'
              ? ` ${t('policy.disableHint', { days: policy.notice_grace_days })}`
              : ''}
          </FieldDescription>
        </Field>
      ))}
      {editable ? (
        <Button type="submit" disabled={update.isPending}>
          {t('policy.save')}
        </Button>
      ) : (
        <p className="text-sm text-muted-foreground">{t('policy.adminOnly')}</p>
      )}
    </form>
  )
}

function AccountApprovalCard() {
  const { t } = useTranslation('security')
  const { data } = useQuery(accountSecurityQueryOptions())
  const update = useUpdateAccountSecurity()
  if (!data) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('account.title')}</CardTitle>
        <CardDescription>{t('account.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {data.reset_flagged ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{t('account.flagged')}</AlertDescription>
          </Alert>
        ) : null}
        <div className="flex items-center justify-between gap-4">
          <p>
            {data.require_new_device_approval
              ? t('account.on')
              : t('account.off')}
          </p>
          <Switch
            checked={data.require_new_device_approval}
            disabled={update.isPending}
            aria-label={t('account.title')}
            onCheckedChange={(next) =>
              update.mutate(next, {
                onError: (e) =>
                  toast.add({
                    title: e.message || t('account.saveError'),
                    type: 'error',
                  }),
              })
            }
          />
        </div>
      </CardContent>
    </Card>
  )
}

export function PolicyPanel() {
  const { t } = useTranslation('security')
  const isRoot = useIsRoot()
  const { data: policy } = useQuery(securityPolicyQueryOptions())
  return (
    <div className="space-y-6">
      <AccountApprovalCard />
      <Card>
        <CardHeader>
          <CardTitle>{t('policy.title')}</CardTitle>
          <CardDescription>{t('policy.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          {policy ? (
            <PolicyForm
              key={JSON.stringify(policy)}
              policy={policy}
              editable={isRoot}
            />
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
