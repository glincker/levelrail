import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FlaskConicalGlassIcon } from '@thesvg/ui-react'
import type { OAuthSSOPolicy, OAuthSSOPolicyInput } from '@/queries/ssoOidc'
import {
  policyInput,
  splitDomains,
  ssoTestStartURL,
  useSaveOAuthSSOPolicy,
} from '@/queries/ssoOidc'
import type { RoleResource } from '@/queries/roles'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { FormSection } from '@/components/ui/form-section'
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

function SwitchRow({
  id,
  label,
  help,
  checked,
  onChange,
}: {
  id: string
  label: string
  help: ReactNode
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <Field>
      <div className="flex items-center justify-between gap-3">
        <FieldLabel htmlFor={id} className="font-normal">
          {label}
        </FieldLabel>
        <Switch id={id} checked={checked} onCheckedChange={onChange} />
      </div>
      <FieldDescription>{help}</FieldDescription>
    </Field>
  )
}

/** OIDCPolicyForm edits the sign-in policy of one OAuth or OIDC provider. */
export function OIDCPolicyForm({
  policy,
  roles,
}: {
  policy: OAuthSSOPolicy
  roles: RoleResource[]
}) {
  const { t } = useTranslation('sso')
  const save = useSaveOAuthSSOPolicy()
  const [form, setForm] = useState<OAuthSSOPolicyInput>(policyInput(policy))
  const [domains, setDomains] = useState(policy.allowed_domains.join(', '))
  const set = <K extends keyof OAuthSSOPolicyInput>(
    key: K,
    value: OAuthSSOPolicyInput[K],
  ) => setForm((f) => ({ ...f, [key]: value }))
  const id = (s: string) => `oidc-${policy.provider}-${s}`
  // A claim can never grant root, so roles that hold it are not offered.
  const grantable = roles.filter((r) => r.id && !r.abilities.includes('root'))

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(
          {
            provider: policy.provider,
            input: { ...form, allowed_domains: splitDomains(domains) },
          },
          {
            onSuccess: () =>
              toast.add({ title: t('oidc.form.saved'), type: 'success' }),
          },
        )
      }}
    >
      <FormSection
        className="lg:grid-cols-1"
        title={t('oidc.form.access.title')}
        description={t('oidc.form.access.description')}
        error={save.isError ? save.error.message : undefined}
      >
        <FieldGroup className="gap-4">
          <SwitchRow
            id={id('jit')}
            label={t('oidc.form.jit')}
            help={t('oidc.form.jitHelp')}
            checked={form.jit_provisioning}
            onChange={(v) => set('jit_provisioning', v)}
          />
          <SwitchRow
            id={id('sync')}
            label={t('oidc.form.syncRoles')}
            help={t('oidc.form.syncRolesHelp')}
            checked={form.sync_roles}
            onChange={(v) => set('sync_roles', v)}
          />
          <Field>
            <FieldLabel htmlFor={id('role')}>
              {t('oidc.form.defaultRole')}
            </FieldLabel>
            <Select
              value={form.default_role_id}
              onValueChange={(v) => set('default_role_id', v ?? '')}
            >
              <SelectTrigger id={id('role')} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {grantable.map((r) => (
                  <SelectItem key={r.id} value={r.id ?? ''}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              {t('oidc.form.defaultRoleHelp')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor={id('domains')}>
              {t('oidc.form.domains')}
            </FieldLabel>
            <Input
              id={id('domains')}
              value={domains}
              onChange={(e) => setDomains(e.target.value)}
            />
            <FieldDescription>{t('oidc.form.domainsHelp')}</FieldDescription>
          </Field>
        </FieldGroup>
      </FormSection>
      <FormSection
        className="lg:grid-cols-1"
        title={t('oidc.form.groups.title')}
        description={t('oidc.form.groups.description')}
      >
        <FieldGroup className="gap-4">
          {policy.groups_supported ? null : (
            <Alert>
              <AlertDescription>
                {t('oidc.form.groups.unsupported')}
              </AlertDescription>
            </Alert>
          )}
          <Field>
            <FieldLabel htmlFor={id('claim')}>
              {t('oidc.form.groupsClaim')}
            </FieldLabel>
            <Input
              id={id('claim')}
              className="font-mono"
              value={form.groups_claim}
              disabled={!policy.groups_supported}
              onChange={(e) => set('groups_claim', e.target.value)}
            />
            <FieldDescription>
              {t('oidc.form.groupsClaimHelp')}
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FormSection>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={save.isPending}>
          {t('oidc.form.save')}
        </Button>
        {policy.enabled ? (
          <Button
            type="button"
            variant="outline"
            render={<a href={ssoTestStartURL(policy.provider)} />}
          >
            <FlaskConicalGlassIcon size="1.2em" aria-hidden="true" />
            {t('oidc.test.button')}
          </Button>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">{t('oidc.test.hint')}</p>
    </form>
  )
}
