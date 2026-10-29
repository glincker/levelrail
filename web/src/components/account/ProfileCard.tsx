import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BadgeCheckGlassIcon,
  PencilGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useSession } from '../../queries/security'
import { useUpdateDisplayName } from '../../queries/myAccess'

const MAX_NAME = 80

export function ProfileCard() {
  const { t } = useTranslation('settings')
  const { data: session } = useSession()
  const update = useUpdateDisplayName()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const editRef = useRef<HTMLButtonElement>(null)
  const inputId = useId()

  const close = () => {
    setEditing(false)
    update.reset()
    window.requestAnimationFrame(() => editRef.current?.focus())
  }

  const save = () => {
    const next = draft.trim()
    if (next === '' || next === session.display_name) {
      close()
      return
    }
    update.mutate(next, {
      onSuccess: () => {
        close()
        toast.add({ title: t('accountPage.profile.saved'), type: 'success' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.profile.title')}</CardTitle>
        <CardDescription>
          {t('accountPage.profile.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="space-y-4 text-sm">
          <div className="space-y-1">
            <dt className="text-xs font-medium text-muted-foreground">
              {t('accountPage.profile.email')}
            </dt>
            <dd className="flex flex-wrap items-center gap-2 text-foreground">
              {session.username}
              <Badge variant={session.email_verified ? 'success' : 'muted'}>
                {session.email_verified ? (
                  <BadgeCheckGlassIcon
                    className="text-tone-success"
                    size="1.4em"
                    aria-hidden="true"
                  />
                ) : null}
                {session.email_verified
                  ? t('accountPage.profile.emailVerified')
                  : t('accountPage.profile.emailUnverified')}
              </Badge>
            </dd>
          </div>
          <div className="space-y-1">
            <dt className="text-xs font-medium text-muted-foreground">
              {editing ? (
                <Label htmlFor={inputId} className="text-xs">
                  {t('accountPage.profile.displayName')}
                </Label>
              ) : (
                t('accountPage.profile.displayName')
              )}
            </dt>
            {editing ? (
              <dd className="space-y-2">
                <form
                  className="flex max-w-md items-center gap-2"
                  onSubmit={(e) => {
                    e.preventDefault()
                    save()
                  }}
                >
                  <Input
                    id={inputId}
                    value={draft}
                    maxLength={MAX_NAME}
                    autoFocus
                    aria-invalid={update.isError}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                  <Button type="submit" size="sm" disabled={update.isPending}>
                    {update.isPending
                      ? t('accountPage.profile.saving')
                      : t('accountPage.profile.save')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={close}
                  >
                    {t('accountPage.profile.cancel')}
                  </Button>
                </form>
                <p className="text-xs text-muted-foreground" aria-live="polite">
                  {t('accountPage.profile.counter', {
                    used: draft.length,
                    max: MAX_NAME,
                  })}
                </p>
                {update.isError ? (
                  <Alert variant="destructive" className="max-w-md">
                    <TriangleAlertGlassIcon
                      className="text-tone-warning"
                      size="1.7em"
                      aria-hidden="true"
                    />
                    <AlertDescription>
                      {t('accountPage.profile.saveError')}
                    </AlertDescription>
                  </Alert>
                ) : null}
              </dd>
            ) : (
              <dd className="flex items-center gap-2 text-foreground">
                {session.display_name}
                <Button
                  ref={editRef}
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-label={t('accountPage.profile.edit')}
                  onClick={() => {
                    setDraft(session.display_name)
                    setEditing(true)
                  }}
                >
                  <PencilGlassIcon size="1.4em" aria-hidden="true" />
                </Button>
              </dd>
            )}
            <dd className="text-xs text-muted-foreground">
              {session.has_password
                ? t('accountPage.profile.hasPassword')
                : t('accountPage.profile.noPassword')}
            </dd>
          </div>
        </dl>
      </CardContent>
    </Card>
  )
}
