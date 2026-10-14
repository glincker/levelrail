import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSession } from '../../queries/security'
import { useUpdateDisplayName } from '../../queries/myAccess'

export function ProfileCard() {
  const { t } = useTranslation('settings')
  const { data: session } = useSession()
  const update = useUpdateDisplayName()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')

  const save = () => {
    const next = draft.trim()
    if (next === '' || next === session.display_name) {
      setEditing(false)
      return
    }
    update.mutate(next, {
      onSuccess: () => {
        setEditing(false)
        toast.add({ title: t('accountPage.profile.saved'), type: 'success' })
      },
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
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
            <dd className="text-foreground">{session.username}</dd>
          </div>
          <div className="space-y-1">
            <dt className="text-xs font-medium text-muted-foreground">
              {t('accountPage.profile.displayName')}
            </dt>
            {editing ? (
              <dd>
                <form
                  className="flex max-w-md items-center gap-2"
                  onSubmit={(e) => {
                    e.preventDefault()
                    save()
                  }}
                >
                  <Input
                    value={draft}
                    maxLength={80}
                    aria-label={t('accountPage.profile.displayName')}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                  <Button type="submit" size="sm" disabled={update.isPending}>
                    {t('accountPage.profile.save')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setEditing(false)}
                  >
                    {t('accountPage.profile.cancel')}
                  </Button>
                </form>
              </dd>
            ) : (
              <dd className="flex items-center gap-2 text-foreground">
                {session.display_name}
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-label={t('accountPage.profile.edit')}
                  onClick={() => {
                    setDraft(session.display_name)
                    setEditing(true)
                  }}
                >
                  <PencilSimpleIcon aria-hidden="true" />
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
