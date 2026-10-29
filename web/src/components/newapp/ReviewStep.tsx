import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  KeyGlassIcon,
  PlusGlassIcon,
  RocketGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { ImportAnalysis } from '../../queries/importAnalyze'
import {
  missingRequiredEnv,
  plannedChanges,
  type ApplyStep,
  type ChangeKind,
  type NewAppEdits,
} from '../../lib/newApp'

const KIND_ICON: Record<ChangeKind, typeof PlusGlassIcon> = {
  app: RocketGlassIcon,
  source: GitBranchGlassIcon,
  database: DatabaseGlassIcon,
  domain: GlobeGlassIcon,
  env: KeyGlassIcon,
}

/**
 * Step 3, "Review and deploy": a plain summary, a diff-like list of what
 * will be created, and one Deploy button.
 */
export function ReviewStep({
  analysis,
  edits,
  deploying,
  failure,
  onBack,
  onDeploy,
}: {
  analysis: ImportAnalysis
  edits: NewAppEdits
  deploying: boolean
  failure: { step: ApplyStep; message: string; appCreated: boolean } | null
  onBack: () => void
  onDeploy: () => void
}) {
  const { t } = useTranslation('newApp')
  const changes = plannedChanges(analysis, edits)
  const missing = missingRequiredEnv(analysis, edits)
  const dbCount = edits.databases.filter((d) => d.enabled).length

  return (
    <div className="space-y-4">
      <p className="text-sm text-foreground">
        {t('review.summary', {
          name: edits.name,
          framework: analysis.stack.framework ?? analysis.stack.runtime,
          databases: dbCount,
          domain: edits.domain || t('review.noDomain'),
        })}
      </p>

      <ul
        className="divide-y divide-border rounded-md border border-border"
        aria-label={t('review.changesLabel')}
      >
        {changes.map((c) => {
          const Icon = KIND_ICON[c.kind]
          return (
            <li key={c.id} className="flex items-start gap-3 px-3 py-2">
              <PlusGlassIcon
                className="mt-0.5 size-5 shrink-0 text-tone-success"
                aria-label={t('review.willCreate')}
              />
              <Icon
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span className="min-w-0 text-sm">
                <span className="font-medium break-all">{c.label}</span>
                {c.detail ? (
                  <span className="block text-xs text-muted-foreground">
                    {t(`review.kinds.${c.kind}`)}: {c.detail}
                  </span>
                ) : (
                  <span className="block text-xs text-muted-foreground">
                    {t(`review.kinds.${c.kind}`)}
                  </span>
                )}
              </span>
            </li>
          )
        })}
      </ul>

      {missing.length > 0 ? (
        <Alert variant="destructive">
          <AlertTitle>{t('review.missingTitle')}</AlertTitle>
          <AlertDescription>
            {t('review.missingBody', { keys: missing.join(', ') })}
          </AlertDescription>
        </Alert>
      ) : null}

      {failure ? (
        <Alert variant="destructive">
          <AlertTitle>
            {t(`review.failed.${failure.step}`, {
              defaultValue: t('review.failed.generic'),
            })}
          </AlertTitle>
          <AlertDescription>
            {failure.message}{' '}
            {failure.appCreated ? t('review.failedResume') : null}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap justify-between gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={deploying}
          onClick={onBack}
        >
          {t('nav.back')}
        </Button>
        <Button
          type="button"
          disabled={deploying || missing.length > 0}
          onClick={onDeploy}
        >
          <RocketGlassIcon size="1.4em" />
          {deploying ? t('review.deploying') : t('review.deploy')}
        </Button>
      </div>
    </div>
  )
}
