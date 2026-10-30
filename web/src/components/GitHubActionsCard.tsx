import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SocialGithubGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useGitHubActionsSetting,
  useSetGitHubActionsSetting,
  type GitHubActionsSetting,
} from '../queries/githubActions'

function GitHubActionsForm({
  appName,
  initial,
}: {
  appName: string
  initial: GitHubActionsSetting
}) {
  const { t } = useTranslation('deploys')
  const save = useSetGitHubActionsSetting(appName)
  const [draft, setDraft] = useState(initial)
  const dirty =
    draft.environment !== initial.environment ||
    draft.report_deployments !== initial.report_deployments ||
    draft.report_checks !== initial.report_checks

  function submit() {
    save.mutate(draft, {
      onSuccess: (saved) => {
        setDraft(saved)
        toast.add({ title: t('githubActions.toast.saved'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('githubActions.toast.errorTitle'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <CardContent className="space-y-4">
      <div className="space-y-1.5">
        <label
          htmlFor="gha-environment"
          className="text-sm font-medium text-foreground"
        >
          {t('githubActions.environmentLabel')}
        </label>
        <Input
          id="gha-environment"
          value={draft.environment}
          onChange={(e) => {
            setDraft({ ...draft, environment: e.target.value })
          }}
        />
      </div>
      <div className="flex items-center justify-between gap-4">
        <p className="text-sm font-medium text-foreground">
          {t('githubActions.deploymentsLabel')}
        </p>
        <Switch
          checked={draft.report_deployments}
          onCheckedChange={(v) => {
            setDraft({ ...draft, report_deployments: v })
          }}
          aria-label={t('githubActions.deploymentsLabel')}
        />
      </div>
      <div className="flex items-center justify-between gap-4">
        <p className="text-sm font-medium text-foreground">
          {t('githubActions.checksLabel')}
        </p>
        <Switch
          checked={draft.report_checks}
          onCheckedChange={(v) => {
            setDraft({ ...draft, report_checks: v })
          }}
          aria-label={t('githubActions.checksLabel')}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {t('githubActions.snippetHint', { app: appName })}
      </p>
      <Button
        type="button"
        onClick={submit}
        disabled={!dirty || save.isPending}
      >
        {t('githubActions.save')}
      </Button>
    </CardContent>
  )
}

// GitHubActionsCard edits what the GitHub Actions deploy action posts back
// to the commit (GET/PUT /api/v1/apps/{name}/github-actions).
export function GitHubActionsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = useGitHubActionsSetting(appName)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SocialGithubGlassIcon className="size-5 text-muted-foreground" />
          {t('githubActions.title')}
          <InfoTip label={t('githubActions.infoTipLabel')}>
            {t('githubActions.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <GitHubActionsForm appName={appName} initial={setting.data} />
    </Card>
  )
}
