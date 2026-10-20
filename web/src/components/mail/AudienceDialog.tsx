import { useEffect, useMemo, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import {
  previewMailAudience,
  useMailAudienceOptions,
  useSaveMailGroup,
  type MailGroup,
  type MailRules,
} from '../../queries/mail'

const EMPTY_RULES: MailRules = {
  all_users: false,
  roles: [],
  teams: [],
  projects: [],
  email_domains: [],
  inactive_days: 0,
  extra_addresses: [],
  exclude_addresses: [],
}

const toLines = (s: string): string[] =>
  s
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')

function toggle(list: string[], value: string, on: boolean): string[] {
  return on ? [...list, value] : list.filter((v) => v !== value)
}

function CheckList({
  label,
  options,
  selected,
  onChange,
}: {
  label: string
  options: { id: string; label: string }[]
  selected: string[]
  onChange: (next: string[]) => void
}) {
  if (options.length === 0) return null
  return (
    <fieldset className="space-y-1.5">
      <legend className="text-sm font-medium">{label}</legend>
      <div className="flex flex-wrap gap-x-4 gap-y-1.5">
        {options.map((o) => (
          <label key={o.id} className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={selected.includes(o.id)}
              onCheckedChange={(on) =>
                onChange(toggle(selected, o.id, on === true))
              }
            />
            {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

/** AudienceDialog creates or edits an audience, with a live recipient count as the rules change. */
export function AudienceDialog({
  group,
  open,
  onOpenChange,
}: {
  group: MailGroup | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('mail')
  const { data: options } = useMailAudienceOptions()
  const save = useSaveMailGroup(group?.id)
  const [name, setName] = useState(group?.name ?? '')
  const [description, setDescription] = useState(group?.description ?? '')
  const [rules, setRules] = useState<MailRules>(group?.rules ?? EMPTY_RULES)
  const [domains, setDomains] = useState(
    (group?.rules.email_domains ?? []).join('\n'),
  )
  const [extra, setExtra] = useState(
    (group?.rules.extra_addresses ?? []).join('\n'),
  )
  const [exclude, setExclude] = useState(
    (group?.rules.exclude_addresses ?? []).join('\n'),
  )
  const [error, setError] = useState<string | null>(null)

  const effective = useMemo<MailRules>(
    () => ({
      ...rules,
      email_domains: toLines(domains),
      extra_addresses: toLines(extra),
      exclude_addresses: toLines(exclude),
    }),
    [rules, domains, extra, exclude],
  )
  const [settled, setSettled] = useState(effective)
  useEffect(() => {
    const id = setTimeout(() => setSettled(effective), 400)
    return () => clearTimeout(id)
  }, [effective])

  const hasRule =
    settled.all_users ||
    settled.roles.length > 0 ||
    settled.teams.length > 0 ||
    settled.projects.length > 0 ||
    settled.email_domains.length > 0 ||
    settled.inactive_days > 0 ||
    settled.extra_addresses.length > 0
  const live = useQuery({
    queryKey: ['mail', 'audience-preview', settled],
    queryFn: () => previewMailAudience(settled),
    enabled: open && hasRule,
    placeholderData: keepPreviousData,
    retry: false,
  })

  const patch = (next: Partial<MailRules>) =>
    setRules((r) => ({ ...r, ...next }))

  const onSave = () => {
    setError(null)
    save.mutate(
      { name: name.trim(), description: description.trim(), rules: effective },
      {
        onSuccess: () => {
          toast.add({ title: t('audiences.dialog.saved'), type: 'success' })
          onOpenChange(false)
        },
        onError: (e) => setError(e.message),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {group
              ? t('audiences.dialog.editTitle')
              : t('audiences.dialog.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('audiences.description')}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="mail-group-name">
              {t('audiences.dialog.name')}
            </FieldLabel>
            <Input
              id="mail-group-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="mail-group-desc">
              {t('audiences.dialog.description')}
            </FieldLabel>
            <Input
              id="mail-group-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </Field>
          <div className="space-y-3">
            <h3 className="text-sm font-medium">
              {t('audiences.dialog.rules')}
            </h3>
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={rules.all_users}
                onCheckedChange={(on) => patch({ all_users: on === true })}
              />
              {t('audiences.dialog.everyone')}
            </label>
            <CheckList
              label={t('audiences.dialog.roles')}
              options={(options?.roles ?? []).map((r) => ({
                id: r,
                label: t(
                  `audiences.role.${r as 'owner' | 'admin' | 'operator' | 'viewer'}`,
                ),
              }))}
              selected={rules.roles}
              onChange={(roles) => patch({ roles })}
            />
            <CheckList
              label={t('audiences.dialog.teams')}
              options={(options?.teams ?? []).map((x) => ({
                id: x.id,
                label: x.name,
              }))}
              selected={rules.teams}
              onChange={(teams) => patch({ teams })}
            />
            <CheckList
              label={t('audiences.dialog.projects')}
              options={(options?.projects ?? []).map((x) => ({
                id: x.id,
                label: x.name,
              }))}
              selected={rules.projects}
              onChange={(projects) => patch({ projects })}
            />
          </div>
          <Field>
            <FieldLabel htmlFor="mail-group-domains">
              {t('audiences.dialog.domains')}
            </FieldLabel>
            <Textarea
              id="mail-group-domains"
              rows={2}
              value={domains}
              onChange={(e) => setDomains(e.target.value)}
            />
            <FieldDescription>
              {t('audiences.dialog.domainsHelp')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="mail-group-inactive">
              {t('audiences.dialog.inactive')}
            </FieldLabel>
            <Input
              id="mail-group-inactive"
              type="number"
              min={0}
              max={3650}
              value={rules.inactive_days === 0 ? '' : rules.inactive_days}
              onChange={(e) =>
                patch({
                  inactive_days: Math.max(0, Number(e.target.value) || 0),
                })
              }
            />
            <FieldDescription>
              {t('audiences.dialog.inactiveHelp')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="mail-group-extra">
              {t('audiences.dialog.extra')}
            </FieldLabel>
            <Textarea
              id="mail-group-extra"
              rows={2}
              value={extra}
              onChange={(e) => setExtra(e.target.value)}
            />
            <FieldDescription>
              {t('audiences.dialog.extraHelp')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="mail-group-exclude">
              {t('audiences.dialog.exclude')}
            </FieldLabel>
            <Textarea
              id="mail-group-exclude"
              rows={2}
              value={exclude}
              onChange={(e) => setExclude(e.target.value)}
            />
            <FieldDescription>
              {t('audiences.dialog.excludeHelp')}
            </FieldDescription>
          </Field>
        </FieldGroup>
        <div
          aria-live="polite"
          className="space-y-1 rounded-lg border border-border bg-muted/40 p-3 text-sm"
        >
          {!hasRule ? (
            <p className="text-muted-foreground">
              {t('audiences.dialog.noneChosen')}
            </p>
          ) : live.data ? (
            <>
              <p className="font-medium">
                {t('audiences.dialog.live', {
                  deliverable: live.data.deliverable,
                  suppressed: live.data.suppressed,
                })}
              </p>
              {live.data.sample.length > 0 ? (
                <p className="text-xs text-muted-foreground">
                  {t('audiences.dialog.liveSample', {
                    count: live.data.sample.length,
                  })}
                  : {live.data.sample.join(', ')}
                </p>
              ) : null}
            </>
          ) : live.isError ? (
            <p className="text-destructive">{live.error.message}</p>
          ) : null}
        </div>
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
            disabled={name.trim() === '' || save.isPending}
            writeAction
            onClick={onSave}
          >
            {save.isPending ? t('common.saving') : t('audiences.dialog.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
