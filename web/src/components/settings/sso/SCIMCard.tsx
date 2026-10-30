import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  KeyGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import type { SCIMToken, SCIMTokenCreated, SSOSettings } from '@/queries/sso'
import {
  useCreateSCIMToken,
  useRevokeSCIMToken,
  useRotateSCIMToken,
  useSetSCIMDefaultRole,
} from '@/queries/sso'
import type { RoleResource } from '@/queries/roles'
import { CopyField } from '@/components/traffic/CopyField'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { FormSection } from '@/components/ui/form-section'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'

/** SCIMCard shows the SCIM endpoint, the directory default role and the tokens. */
export function SCIMCard({
  settings,
  tokens,
  roles,
}: {
  settings: SSOSettings
  tokens: SCIMToken[]
  roles: RoleResource[]
}) {
  const { t, i18n } = useTranslation('sso')
  const create = useCreateSCIMToken()
  const rotate = useRotateSCIMToken()
  const revoke = useRevokeSCIMToken()
  const setRole = useSetSCIMDefaultRole()
  const [name, setName] = useState('')
  const [shown, setShown] = useState<SCIMTokenCreated | null>(null)
  const [revoking, setRevoking] = useState<SCIMToken | null>(null)
  const dateFmt = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  const error = create.error ?? rotate.error ?? setRole.error

  return (
    <FormSection
      title={t('scim.title')}
      description={t('scim.description')}
      error={error?.message}
    >
      <FieldGroup className="gap-4">
        <Field>
          <FieldLabel>{t('scim.baseUrl')}</FieldLabel>
          <CopyField value={settings.scim_base_url} label={t('scim.baseUrl')} />
        </Field>
        <Field>
          <FieldLabel htmlFor="scim-default-role">
            {t('scim.defaultRole')}
          </FieldLabel>
          <Select
            value={settings.scim_default_role_id}
            onValueChange={(v) => {
              if (!v) return
              setRole.mutate(v, {
                onSuccess: () =>
                  toast.add({ title: t('scim.roleSaved'), type: 'success' }),
              })
            }}
          >
            <SelectTrigger id="scim-default-role" className="w-full sm:w-80">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {roles
                .filter((r) => r.id)
                .map((r) => (
                  <SelectItem key={r.id} value={r.id ?? ''}>
                    {r.name}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
          <FieldDescription>{t('scim.defaultRoleHelp')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="scim-token-name">
            {t('scim.tokenName')}
          </FieldLabel>
          <form
            className="flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              create.mutate(name.trim(), {
                onSuccess: (tok) => {
                  setName('')
                  setShown(tok)
                },
              })
            }}
          >
            <Input
              id="scim-token-name"
              className="min-w-0 flex-1"
              placeholder={t('scim.tokenNamePlaceholder')}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              <KeyGlassIcon size="1.4em" aria-hidden="true" />
              {t('scim.create')}
            </Button>
          </form>
        </Field>
      </FieldGroup>
      {tokens.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('scim.empty')}</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border bg-card px-4">
          {tokens.map((tok) => (
            <li
              key={tok.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3"
            >
              <span className="min-w-0 flex-1 truncate text-sm font-medium">
                {tok.name}
              </span>
              <code className="font-mono text-xs text-muted-foreground">
                {tok.prefix}
              </code>
              <span className="text-xs text-muted-foreground">
                {tok.last_used_at
                  ? t('scim.lastUsed', {
                      date: dateFmt.format(new Date(tok.last_used_at)),
                    })
                  : t('scim.neverUsed')}
              </span>
              {tok.revoked_at ? (
                <Badge variant="muted">{t('scim.revoked')}</Badge>
              ) : (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={rotate.isPending}
                    onClick={() =>
                      rotate.mutate(tok.id, { onSuccess: setShown })
                    }
                  >
                    <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
                    {t('scim.rotate')}
                  </Button>
                  <Button
                    type="button"
                    variant="destructive"
                    size="sm"
                    onClick={() => setRevoking(tok)}
                  >
                    <BanGlassIcon size="1.4em" aria-hidden="true" />
                    {t('scim.revoke')}
                  </Button>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
      <Dialog
        open={shown !== null}
        onOpenChange={(open) => {
          if (!open) setShown(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('scim.shownOnceTitle')}</DialogTitle>
            <DialogDescription>{t('scim.shownOnceBody')}</DialogDescription>
          </DialogHeader>
          {shown ? (
            <CopyField value={shown.token} label={t('scim.shownOnceTitle')} />
          ) : null}
          <DialogFooter>
            <Button type="button" onClick={() => setShown(null)}>
              {t('scim.done')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => {
          if (!open) setRevoking(null)
        }}
        title={t('scim.revokeTitle', { name: revoking?.name ?? '' })}
        description={t('scim.revokeBody')}
        confirmLabel={t('scim.revoke')}
        tone="destructive"
        pending={revoke.isPending}
        error={revoke.isError ? revoke.error.message : null}
        onConfirm={() => {
          if (!revoking) return
          revoke.mutate(revoking.id, {
            onSuccess: () => {
              toast.add({ title: t('scim.revokedToast'), type: 'success' })
              setRevoking(null)
            },
          })
        }}
      />
    </FormSection>
  )
}
