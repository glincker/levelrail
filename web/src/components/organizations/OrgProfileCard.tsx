import { useId, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ApiError } from '../../lib/apiError'
import type { OrganizationResource } from '../../types/organization'
import {
  ORG_DESCRIPTION_MAX,
  useUpdateOrganization,
} from '../../queries/organizationProfile'
import { OrgAvatarUploader } from './OrgAvatarUploader'
import { ORG_SLUG_MAX, ORG_SLUG_MIN, slugProblem } from './orgSlug'

export interface OrgProfileCardProps {
  organization: OrganizationResource
  canManage: boolean
  // The default org cannot carry a slug.
  isDefault: boolean
}

// Profile form: org admins edit, everyone else reads.
export function OrgProfileCard({
  organization,
  canManage,
  isDefault,
}: OrgProfileCardProps) {
  const { t } = useTranslation('settings')
  const uid = useId()
  const update = useUpdateOrganization(organization.id)
  const [name, setName] = useState(organization.name)
  const [slug, setSlug] = useState(organization.slug ?? '')
  const [description, setDescription] = useState(organization.description ?? '')
  const [saved, setSaved] = useState(false)

  const slugChanged = slug !== (organization.slug ?? '')
  const problem = slugChanged && slug !== '' ? slugProblem(slug) : null
  const serverErr = update.error instanceof ApiError ? update.error : null
  const slugServerMsg =
    serverErr?.status === 409
      ? t('organizationHome.profile.slugTaken')
      : serverErr?.status === 400 && slugChanged
        ? serverErr.message
        : ''
  const slugMsg =
    problem === 'length'
      ? t('organizationHome.profile.slugLength', {
          min: ORG_SLUG_MIN,
          max: ORG_SLUG_MAX,
        })
      : problem === 'chars'
        ? t('organizationHome.profile.slugChars')
        : slugServerMsg
  const dirty =
    name.trim() !== organization.name ||
    slugChanged ||
    description.trim() !== (organization.description ?? '')
  const valid =
    name.trim() !== '' &&
    problem === null &&
    description.length <= ORG_DESCRIPTION_MAX
  const otherErr = slugServerMsg ? '' : (update.error?.message ?? '')

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!canManage || !dirty || !valid) return
    setSaved(false)
    update.mutate(
      {
        name: name.trim(),
        description: description.trim(),
        ...(slugChanged && slug !== '' ? { slug } : {}),
      },
      { onSuccess: () => setSaved(true) },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('organizationHome.profile.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {canManage ? (
          <OrgAvatarUploader
            id={organization.id}
            name={organization.name}
            avatarUrl={organization.avatar_url}
          />
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('organizationHome.profile.readOnly')}
          </p>
        )}
        <form
          onSubmit={submit}
          noValidate
          className="grid gap-3 md:grid-cols-2"
        >
          <div className="space-y-1.5">
            <Label htmlFor={`${uid}-name`}>
              {t('organizationHome.profile.name')}
            </Label>
            <Input
              id={`${uid}-name`}
              value={name}
              readOnly={!canManage}
              maxLength={100}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor={`${uid}-slug`}>
              {t('organizationHome.profile.slug')}
            </Label>
            <Input
              id={`${uid}-slug`}
              value={slug}
              readOnly={!canManage}
              disabled={isDefault}
              placeholder={t('organizationHome.profile.slugPlaceholder')}
              aria-invalid={slugMsg !== ''}
              aria-describedby={`${uid}-slug-msg`}
              onChange={(e) => {
                update.reset()
                setSlug(e.target.value.toLowerCase())
              }}
            />
            <p
              id={`${uid}-slug-msg`}
              role={slugMsg ? 'alert' : undefined}
              className={
                slugMsg
                  ? 'text-xs text-tone-danger-fg'
                  : 'text-xs text-muted-foreground'
              }
            >
              {slugMsg ||
                (isDefault
                  ? t('organizationHome.profile.slugDefault')
                  : t('organizationHome.profile.slugHint'))}
            </p>
          </div>
          <div className="space-y-1.5 md:col-span-2">
            <Label htmlFor={`${uid}-desc`}>
              {t('organizationHome.profile.description')}
            </Label>
            <Textarea
              id={`${uid}-desc`}
              value={description}
              rows={3}
              readOnly={!canManage}
              maxLength={ORG_DESCRIPTION_MAX}
              onChange={(e) => setDescription(e.target.value)}
            />
            <p className="text-right text-xs text-muted-foreground">
              {description.length}/{ORG_DESCRIPTION_MAX}
            </p>
          </div>
          {canManage ? (
            <div className="flex flex-wrap items-center gap-3 md:col-span-2">
              <Button
                type="submit"
                size="sm"
                disabled={!dirty || !valid || update.isPending}
              >
                {update.isPending
                  ? t('organizationHome.profile.saving')
                  : t('organizationHome.profile.save')}
              </Button>
              {saved && !dirty ? (
                <span role="status" className="text-xs text-tone-success-fg">
                  {t('organizationHome.profile.saved')}
                </span>
              ) : null}
              {otherErr ? (
                <span role="alert" className="text-xs text-tone-danger-fg">
                  {otherErr}
                </span>
              ) : null}
            </div>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
