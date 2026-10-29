import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, SquarePenGlassIcon } from '@thesvg/ui-react'
import type { DeployCompare } from '../types/deployCompare'
import { useReleaseNotes } from '../queries/releaseNotes'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { toast } from '@/components/ui/toast'

export function DeployCompareCommitsCard({
  appName,
  compare,
}: {
  appName: string
  compare: DeployCompare
}) {
  const { t } = useTranslation('deploys')
  const [requested, setRequested] = useState(false)
  const fromId = compare.from.deploy_id
  const toId = compare.to.deploy_id
  const canGenerate = Boolean(fromId && compare.from.commit_sha)
  const notes = useReleaseNotes(
    appName,
    fromId ?? '',
    toId,
    requested && canGenerate,
  )
  const commits = compare.commits ?? []
  const fromDigest = compare.from.image_digest
  const toDigest = compare.to.image_digest

  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.add({ title: t('releaseNotes.copied'), type: 'success' })
    } catch {
      toast.add({ title: t('releaseNotes.copyFailed'), type: 'error' })
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('releaseNotes.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {fromDigest || toDigest ? (
          <p className="break-all font-mono text-xs text-muted-foreground">
            {t('releaseNotes.digest')}: {fromDigest || '-'} {'->'}{' '}
            {toDigest || '-'}
            {fromDigest === toDigest
              ? ` (${t('releaseNotes.digestSame')})`
              : ''}
          </p>
        ) : null}

        {commits.length > 0 ? (
          <>
            <p className="text-sm text-muted-foreground">
              {t('releaseNotes.commitsCount', { count: commits.length })}
            </p>
            <ul className="space-y-1">
              {commits.map((c) => (
                <li key={c.sha} className="flex gap-2 text-sm">
                  <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                    {c.sha.slice(0, 7)}
                  </span>
                  <span className="truncate">{c.message.split('\n')[0]}</span>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            {compare.commits_note || t('releaseNotes.noCommits')}
          </p>
        )}

        {canGenerate ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRequested(true)}
            disabled={notes.isFetching}
          >
            <SquarePenGlassIcon className="size-4" data-icon="inline-start" />
            {notes.isFetching
              ? t('releaseNotes.generating')
              : t('releaseNotes.generate')}
          </Button>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('releaseNotes.needsDeploy')}
          </p>
        )}

        {notes.isError ? (
          <p className="text-sm text-destructive">
            {notes.error instanceof Error
              ? notes.error.message
              : t('releaseNotes.loadFailed')}
          </p>
        ) : null}

        {notes.data ? (
          <div className="space-y-2">
            <CodeBlock
              code={notes.data.markdown}
              language="markdown"
              wrap
              maxHeight="lg"
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => void copy(notes.data.markdown)}
            >
              <CopyGlassIcon className="size-4" data-icon="inline-start" />
              {t('releaseNotes.copy')}
            </Button>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
