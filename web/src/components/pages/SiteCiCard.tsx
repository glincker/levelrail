import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { InfoTip } from '@/components/kit/InfoTip'
import {
  useCreateSiteDeployToken,
  useRevokeSiteDeployToken,
  useSiteDeployTokens,
} from '../../queries/pagesCiTokens'
import { ciSnippets } from './ciSnippets'

type Snippet = keyof ReturnType<typeof ciSnippets>

/** Deploy-only tokens for one site plus copyable CI snippets. The secret is held in state only. */
export default function SiteCiCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = useSiteDeployTokens(name)
  const create = useCreateSiteDeployToken(name)
  const revoke = useRevokeSiteDeployToken(name)
  const [shown, setShown] = useState('')
  const [copied, setCopied] = useState('')
  const snippets = ciSnippets(name, window.location.origin)
  const active = (q.data ?? []).filter((x) => !x.revoked_at)
  const error = create.error ?? revoke.error

  function copy(id: string, text: string) {
    void navigator.clipboard.writeText(text).then(() => setCopied(id))
  }

  return (
    <section
      aria-labelledby="pages-ci-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2
          id="pages-ci-title"
          className="flex items-center gap-1 text-sm font-semibold"
        >
          {t('ci.title')}
          <InfoTip>{t('ci.tip')}</InfoTip>
        </h2>
        <Button
          size="sm"
          variant="outline"
          disabled={create.isPending}
          title={t('ci.createTip')}
          onClick={() =>
            create.mutate('ci', {
              onSuccess: (r) => {
                setShown(r.token ?? '')
                setCopied('')
              },
            })
          }
        >
          {t('ci.create')}
        </Button>
      </div>
      {q.isPending ? <Skeleton className="h-6 w-full" /> : null}
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('ci.failed')}</AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
      {shown ? (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 text-xs">
              {shown}
            </code>
            <Button
              size="sm"
              variant="outline"
              onClick={() => copy('token', shown)}
            >
              {copied === 'token' ? t('ci.copied') : t('ci.copy')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t('ci.shownOnce')}</p>
        </div>
      ) : null}
      {active.length > 0 ? (
        <ul className="space-y-1">
          {active.map((tok) => (
            <li
              key={tok.id}
              className="flex items-center justify-between gap-2 text-xs"
            >
              <span className="min-w-0 truncate">
                {tok.name}
                {tok.expires_at
                  ? ` · ${t('ci.expires', { date: new Date(tok.expires_at).toLocaleDateString() })}`
                  : ''}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={revoke.isPending}
                title={t('ci.revokeTip')}
                onClick={() => {
                  if (window.confirm(t('ci.confirmRevoke'))) {
                    setShown('')
                    revoke.mutate(tok.id)
                  }
                }}
              >
                {t('ci.revoke')}
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
      <Tabs defaultValue="github">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="github">{t('ci.github')}</TabsTrigger>
          <TabsTrigger value="gitlab">{t('ci.gitlab')}</TabsTrigger>
          <TabsTrigger value="curl">{t('ci.curl')}</TabsTrigger>
        </TabsList>
        {(Object.keys(snippets) as Snippet[]).map((k) => (
          <TabsContent key={k} value={k} className="space-y-1 pt-2">
            <pre className="overflow-x-auto rounded bg-muted p-2 text-xs">
              {snippets[k]}
            </pre>
            <Button
              size="sm"
              variant="outline"
              onClick={() => copy(k, snippets[k])}
            >
              {copied === k ? t('ci.copied') : t('ci.copy')}
            </Button>
          </TabsContent>
        ))}
      </Tabs>
    </section>
  )
}
