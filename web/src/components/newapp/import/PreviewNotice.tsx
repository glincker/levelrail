import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { PreviewKind } from '../../../queries/repoPreview'

const CONNECT_ROUTE: Record<string, string> = {
  github: '/settings/github-app',
  gitlab: '/settings/gitlab-app',
  bitbucket: '/settings/bitbucket-app',
  gitea: '/settings/gitea-app',
}

type NoticeKind = Exclude<PreviewKind, 'public'>

/** Non-card outcomes: each says what happened in one line and what to do next. */
export function PreviewNotice({
  kind,
  brand,
  onToken,
  onContinue,
}: {
  kind: NoticeKind
  brand: string
  onToken: (token: string) => void
  onContinue: () => void
}) {
  const { t } = useTranslation('newApp')
  const [asking, setAsking] = useState(false)
  const [token, setToken] = useState('')
  const connectTo = CONNECT_ROUTE[brand] ?? CONNECT_ROUTE.github
  return (
    <Alert variant={kind === 'rate_limited' ? 'default' : 'destructive'}>
      <AlertDescription className="space-y-3">
        <p>{t(`import.notice.${kind}`)}</p>
        <div className="flex flex-wrap gap-2">
          {kind === 'private_needs_account' ? (
            <>
              <Button
                size="sm"
                render={<Link to={connectTo} />}
                nativeButton={false}
              >
                {t('import.notice.connect')}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setAsking((v) => !v)
                }}
              >
                {t('import.notice.useToken')}
              </Button>
            </>
          ) : null}
          {kind === 'rate_limited' || kind === 'unsupported_host' ? (
            <Button size="sm" variant="outline" onClick={onContinue}>
              {t('import.notice.continue')}
            </Button>
          ) : null}
        </div>
        {asking ? (
          <form
            noValidate
            className="flex min-w-0 flex-col gap-2 sm:flex-row"
            onSubmit={(e) => {
              e.preventDefault()
              if (token.trim() !== '') onToken(token.trim())
            }}
          >
            <Input
              type="password"
              autoComplete="off"
              value={token}
              aria-label={t('import.notice.tokenLabel')}
              placeholder={t('import.notice.tokenLabel')}
              onChange={(e) => {
                setToken(e.target.value)
              }}
              className="min-w-0 sm:max-w-xs"
            />
            <Button type="submit" size="sm" disabled={token.trim() === ''}>
              {t('import.notice.tokenApply')}
            </Button>
          </form>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
