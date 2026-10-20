import { useState } from 'react'
import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { AbilitiesField } from '../AbilitiesField'
import { useCreateRole, useUpdateRole } from '../../queries/roles'
import type { RoleResource, RoleVisibility } from '../../queries/roles'
import type { Ability } from '../../types/token'

export function RoleFormDialog({
  trigger,
  role,
}: {
  trigger: ReactElement
  role?: RoleResource
}) {
  const { t } = useTranslation('access')
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(role?.name ?? '')
  const [description, setDescription] = useState(role?.description ?? '')
  const [visibility, setVisibility] = useState<RoleVisibility>(
    role?.visibility ?? 'all',
  )
  const [abilities, setAbilities] = useState<Ability[]>(
    role?.abilities ?? ['read'],
  )
  const [submitted, setSubmitted] = useState(false)
  const create = useCreateRole()
  const update = useUpdateRole()
  const mutation = role?.id ? update : create

  const effectiveAbilities: Ability[] =
    visibility === 'granted' ? ['read'] : abilities
  const nameMissing = name.trim() === ''
  const abilitiesMissing = effectiveAbilities.length === 0

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setName(role?.name ?? '')
      setDescription(role?.description ?? '')
      setVisibility(role?.visibility ?? 'all')
      setAbilities(role?.abilities ?? ['read'])
      setSubmitted(false)
    } else {
      create.reset()
      update.reset()
    }
  }

  function handleSave() {
    setSubmitted(true)
    if (nameMissing || abilitiesMissing) {
      return
    }
    const input = {
      name: name.trim(),
      description,
      abilities: effectiveAbilities,
      visibility,
    }
    const onSuccess = (saved: RoleResource) => {
      setOpen(false)
      toast.add({
        title: t(role?.id ? 'roles.toast.updated' : 'roles.toast.created', {
          name: saved.name,
        }),
        type: 'success',
      })
    }
    if (role?.id) {
      update.mutate({ id: role.id, input }, { onSuccess })
    } else {
      create.mutate(input, { onSuccess })
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {role
              ? t('roles.form.editTitle', { name: role.name })
              : t('roles.form.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('roles.form.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="role-name">{t('roles.form.name')}</FieldLabel>
            <Input
              id="role-name"
              value={name}
              placeholder={t('roles.form.namePlaceholder')}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
            {submitted && nameMissing ? (
              <FieldError
                errors={[{ message: t('roles.form.nameRequired') }]}
              />
            ) : null}
          </Field>
          <Field>
            <FieldLabel htmlFor="role-description">
              {t('roles.form.descriptionLabel')}
            </FieldLabel>
            <Input
              id="role-description"
              value={description}
              onChange={(e) => {
                setDescription(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="role-visibility">
              {t('roles.form.visibility')}
            </FieldLabel>
            <Select
              value={visibility}
              onValueChange={(next) => {
                setVisibility(next === 'granted' ? 'granted' : 'all')
              }}
            >
              <SelectTrigger id="role-visibility" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t('roles.form.visibilityAll')}
                </SelectItem>
                <SelectItem value="granted">
                  {t('roles.form.visibilityGranted')}
                </SelectItem>
              </SelectContent>
            </Select>
            {visibility === 'granted' ? (
              <FieldDescription className="text-xs">
                {t('roles.form.visibilityGrantedHint')}
              </FieldDescription>
            ) : null}
          </Field>
          {visibility === 'all' ? (
            <AbilitiesField
              value={abilities}
              onChange={setAbilities}
              error={
                submitted && abilitiesMissing
                  ? { message: t('roles.form.abilitiesRequired') }
                  : undefined
              }
            />
          ) : null}
        </div>
        {mutation.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{mutation.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('roles.form.cancel')}
          </Button>
          <Button
            type="button"
            disabled={mutation.isPending}
            onClick={handleSave}
          >
            {mutation.isPending ? t('roles.form.saving') : t('roles.form.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
