import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { HouseGlassIcon, LockGlassIcon, UsersGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// A 403 with a way forward: switch team, ask an admin from the Teams page,
// or go home. The server's own message says which rule refused.
export function ForbiddenState({ message }: { message?: string }) {
  const { t } = useTranslation('teams')
  return (
    <div className="flex min-h-[60vh] items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-center gap-2">
            <LockGlassIcon
              className="size-5.5 text-muted-foreground"
              aria-hidden="true"
            />
            <CardTitle>{t('forbidden.title')}</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {message ?? t('forbidden.body')}
          </p>
          <div className="flex flex-wrap gap-2">
            <Link to="/settings/teams">
              <Button type="button" variant="outline" size="sm">
                <UsersGlassIcon size="1.2em" aria-hidden="true" />
                {t('forbidden.teams')}
              </Button>
            </Link>
            <Link to="/">
              <Button type="button" variant="ghost" size="sm">
                <HouseGlassIcon size="1.2em" aria-hidden="true" />
                {t('forbidden.home')}
              </Button>
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
