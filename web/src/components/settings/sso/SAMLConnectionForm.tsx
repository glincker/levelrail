import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FlaskConicalGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import type { SAMLConnection, SAMLConnectionInput } from '@/queries/sso'
import {
  connectionInput,
  samlTestStartURL,
  useDeleteSAMLConnection,
  useSaveSAMLConnection,
} from '@/queries/sso'
import type { RoleResource } from '@/queries/roles'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
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
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { SAMLMetadataImport } from './SAMLMetadataImport'
import { SAMLSPDetails } from './SAMLSPDetails'

const NO_ROLE = '__none__'

const EMPTY: SAMLConnectionInput = {
  name: '',
  enabled: false,
  idp_entity_id: '',
  idp_sso_url: '',
  idp_certificates: '',
  idp_metadata_url: '',
  email_attribute: '',
  name_attribute: '',
  groups_attribute: '',
  allowed_domains: [],
  jit_provisioning: false,
  default_role_id: '',
  sync_roles: false,
}

function TextField({
  id,
  label,
  help,
  value,
  onChange,
  mono = false,
}: {
  id: string
  label: string
  help?: ReactNode
  value: string
  onChange: (v: string) => void
  mono?: boolean
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        value={value}
        className={mono ? 'font-mono' : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
      {help ? <FieldDescription>{help}</FieldDescription> : null}
    </Field>
  )
}

function SwitchRow({
  id,
  label,
  help,
  checked,
  onChange,
}: {
  id: string
  label: string
  help?: string
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
      {help ? <FieldDescription>{help}</FieldDescription> : null}
    </Field>
  )
}

/** SAMLConnectionForm creates or edits one SAML connection inside its sheet. */
export function SAMLConnectionForm({
  connection,
  roles,
  onSaved,
  onDeleted,
}: {
  connection?: SAMLConnection
  roles: RoleResource[]
  onSaved: (c: SAMLConnection) => void
  onDeleted: () => void
}) {
  const { t } = useTranslation('sso')
  const save = useSaveSAMLConnection()
  const remove = useDeleteSAMLConnection()
  const [form, setForm] = useState<SAMLConnectionInput>(
    connection ? connectionInput(connection) : EMPTY,
  )
  const [domains, setDomains] = useState(form.allowed_domains.join(', '))
  const [confirmDelete, setConfirmDelete] = useState(false)
  const set = <K extends keyof SAMLConnectionInput>(
    key: K,
    value: SAMLConnectionInput[K],
  ) => setForm((f) => ({ ...f, [key]: value }))
  const id = (s: string) => `saml-${connection?.id ?? 'new'}-${s}`

  const submit = () => {
    const input: SAMLConnectionInput = {
      ...form,
      allowed_domains: domains
        .split(',')
        .map((d) => d.trim())
        .filter(Boolean),
    }
    save.mutate(
      { id: connection?.id, input },
      {
        onSuccess: (saved) => {
          toast.add({ title: t('form.saved'), type: 'success' })
          onSaved(saved)
        },
      },
    )
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <FieldGroup className="gap-4 pb-4">
        <TextField
          id={id('name')}
          label={t('form.name')}
          help={t('form.nameHelp')}
          value={form.name}
          onChange={(v) => set('name', v)}
        />
        <SwitchRow
          id={id('enabled')}
          label={t('form.enabled')}
          checked={form.enabled}
          onChange={(v) => set('enabled', v)}
        />
      </FieldGroup>
      <SAMLSPDetails connection={connection} />
      {connection ? (
        <SAMLMetadataImport
          connectionId={connection.id}
          onImported={(c) => {
            setForm((f) => ({
              ...f,
              idp_entity_id: c.idp_entity_id,
              idp_sso_url: c.idp_sso_url,
              idp_certificates: c.idp_certificates,
              idp_metadata_url: c.idp_metadata_url,
            }))
            onSaved(c)
          }}
        />
      ) : null}
      <FormSection
        className="lg:grid-cols-1"
        title={t('form.idp.title')}
        description={t('form.idp.description')}
      >
        <FieldGroup className="gap-4">
          <TextField
            id={id('entity')}
            label={t('form.idp.entityId')}
            value={form.idp_entity_id}
            onChange={(v) => set('idp_entity_id', v)}
            mono
          />
          <TextField
            id={id('sso')}
            label={t('form.idp.ssoUrl')}
            value={form.idp_sso_url}
            onChange={(v) => set('idp_sso_url', v)}
            mono
          />
          <Field>
            <FieldLabel htmlFor={id('certs')}>
              {t('form.idp.certificates')}
            </FieldLabel>
            <Textarea
              id={id('certs')}
              rows={6}
              className="font-mono text-xs"
              value={form.idp_certificates}
              onChange={(e) => set('idp_certificates', e.target.value)}
            />
            <FieldDescription>
              {t('form.idp.certificatesHelp')}
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FormSection>
      <FormSection
        className="lg:grid-cols-1"
        title={t('form.access.title')}
        description={t('form.access.description')}
      >
        <FieldGroup className="gap-4">
          <SwitchRow
            id={id('jit')}
            label={t('form.access.jit')}
            help={t('form.access.jitHelp')}
            checked={form.jit_provisioning}
            onChange={(v) => set('jit_provisioning', v)}
          />
          <SwitchRow
            id={id('sync')}
            label={t('form.access.syncRoles')}
            help={t('form.access.syncRolesHelp')}
            checked={form.sync_roles}
            onChange={(v) => set('sync_roles', v)}
          />
          <Field>
            <FieldLabel htmlFor={id('role')}>
              {t('form.access.defaultRole')}
            </FieldLabel>
            <Select
              value={form.default_role_id || NO_ROLE}
              onValueChange={(v) =>
                set('default_role_id', v === NO_ROLE || !v ? '' : v)
              }
            >
              <SelectTrigger id={id('role')} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_ROLE}>
                  {t('form.access.defaultRoleNone')}
                </SelectItem>
                {roles
                  .filter((r) => r.id)
                  .map((r) => (
                    <SelectItem key={r.id} value={r.id ?? ''}>
                      {r.name}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </Field>
          <TextField
            id={id('domains')}
            label={t('form.access.domains')}
            help={t('form.access.domainsHelp')}
            value={domains}
            onChange={setDomains}
          />
        </FieldGroup>
      </FormSection>
      <FormSection
        className="lg:grid-cols-1"
        title={t('form.attributes.title')}
        description={t('form.attributes.description')}
      >
        <FieldGroup className="gap-4">
          <TextField
            id={id('email-attr')}
            label={t('form.attributes.email')}
            value={form.email_attribute}
            onChange={(v) => set('email_attribute', v)}
            mono
          />
          <TextField
            id={id('name-attr')}
            label={t('form.attributes.name')}
            value={form.name_attribute}
            onChange={(v) => set('name_attribute', v)}
            mono
          />
          <TextField
            id={id('groups-attr')}
            label={t('form.attributes.groups')}
            value={form.groups_attribute}
            onChange={(v) => set('groups_attribute', v)}
            mono
          />
        </FieldGroup>
      </FormSection>
      {save.isError ? (
        <p role="alert" className="text-sm text-tone-danger">
          {save.error.message}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
        {connection ? (
          <Button
            type="button"
            variant="destructive"
            onClick={() => setConfirmDelete(true)}
          >
            <TrashGlassIcon size="1.4em" aria-hidden="true" />
            {t('form.delete')}
          </Button>
        ) : (
          <span />
        )}
        <div className="flex flex-wrap items-center gap-2">
          {connection?.configured && connection.enabled ? (
            <Button
              type="button"
              variant="outline"
              render={<a href={samlTestStartURL(connection.id)} />}
            >
              <FlaskConicalGlassIcon size="1.4em" aria-hidden="true" />
              {t('saml.test.button')}
            </Button>
          ) : null}
          <Button type="submit" disabled={save.isPending}>
            {t('form.save')}
          </Button>
        </div>
      </div>
      {connection?.configured && connection.enabled ? (
        <p className="text-xs text-muted-foreground">{t('saml.test.hint')}</p>
      ) : null}
      {connection ? (
        <ConfirmDialog
          open={confirmDelete}
          onOpenChange={setConfirmDelete}
          title={t('form.deleteTitle', { name: connection.name })}
          description={t('form.deleteBody')}
          confirmLabel={t('form.delete')}
          tone="destructive"
          pending={remove.isPending}
          error={remove.isError ? remove.error.message : null}
          onConfirm={() =>
            remove.mutate(connection.id, {
              onSuccess: () => {
                toast.add({ title: t('form.deleted'), type: 'success' })
                setConfirmDelete(false)
                onDeleted()
              },
            })
          }
        />
      ) : null}
    </form>
  )
}
