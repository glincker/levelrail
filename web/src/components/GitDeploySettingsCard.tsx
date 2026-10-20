import { useState } from 'react'
import { FunnelIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../queries/gitSources'
import { useSetGitDeploySettings } from '../queries/gitDeploySettings'
import type { GitDeploySettings } from '../types/gitSource'
import { InfoTip } from './kit'

function toLines(list: string[]): string {
  return list.join('\n')
}

function fromLines(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')
}

// GitDeploySettingsCard sets which pushes deploy by the files they changed,
// and whether deploy state is posted back to the git provider. It renders
// only once a git source is connected.
export function GitDeploySettingsCard({ appName }: { appName: string }) {
  const source = useGitSource(appName)
  if (!source.data) {
    return null
  }
  const current: GitDeploySettings = {
    deploy_paths: source.data.deploy_paths ?? [],
    deploy_paths_ignore: source.data.deploy_paths_ignore ?? [],
    report_status: source.data.report_status ?? true,
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FunnelIcon className="size-4 text-muted-foreground" />
          Deploy filters and status reporting
          <InfoTip label="About deploy filters">
            Skip deploys for pushes that only touch files you do not ship, and
            choose whether the git provider sees this app's deploys.
          </InfoTip>
        </CardTitle>
        <CardDescription>
          Path filters apply to pushes. When a push carries no file list, the
          changed files are read from the git provider; if that fails the push
          deploys anyway.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <DeploySettingsForm
          key={JSON.stringify(current)}
          appName={appName}
          current={current}
        />
      </CardContent>
    </Card>
  )
}

function DeploySettingsForm({
  appName,
  current,
}: {
  appName: string
  current: GitDeploySettings
}) {
  const save = useSetGitDeploySettings(appName)
  const [paths, setPaths] = useState(toLines(current.deploy_paths))
  const [ignore, setIgnore] = useState(toLines(current.deploy_paths_ignore))
  const [report, setReport] = useState(current.report_status)

  const dirty =
    paths !== toLines(current.deploy_paths) ||
    ignore !== toLines(current.deploy_paths_ignore) ||
    report !== current.report_status

  const submit = () =>
    save.mutate(
      {
        deploy_paths: fromLines(paths),
        deploy_paths_ignore: fromLines(ignore),
        report_status: report,
      },
      {
        onSuccess: () =>
          toast.add({ title: 'Deploy settings saved.', type: 'success' }),
        onError: (e) =>
          toast.add({
            title: 'Could not save deploy settings.',
            description: e.message,
            type: 'error',
          }),
      },
    )

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="deploy-paths" className="flex items-center gap-1.5">
            Only deploy for changes to
            <InfoTip label="About deploy paths">
              One glob per line, such as src/** or Dockerfile. A push deploys
              only when a changed file matches at least one. Leave empty to
              deploy every push.
            </InfoTip>
          </Label>
          <Textarea
            id="deploy-paths"
            value={paths}
            onChange={(e) => setPaths(e.target.value)}
            rows={3}
            spellCheck={false}
            className="font-mono text-xs"
            placeholder="src/**"
          />
        </div>
        <div className="space-y-1.5">
          <Label
            htmlFor="deploy-paths-ignore"
            className="flex items-center gap-1.5"
          >
            Skip when only these change
            <InfoTip label="About ignored paths">
              One glob per line, such as docs/** or **/*.md. A push is skipped
              when every changed file matches. The skip reason shows in webhook
              deliveries.
            </InfoTip>
          </Label>
          <Textarea
            id="deploy-paths-ignore"
            value={ignore}
            onChange={(e) => setIgnore(e.target.value)}
            rows={3}
            spellCheck={false}
            className="font-mono text-xs"
            placeholder="docs/**"
          />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Switch
          id="deploy-report-status"
          checked={report}
          onCheckedChange={setReport}
          aria-label="Report deploys and pipeline runs to the git provider"
        />
        <Label
          htmlFor="deploy-report-status"
          className="flex items-center gap-1.5"
        >
          Report to the git provider
          <InfoTip label="About status reporting">
            Posts pipeline run states as commit statuses and, on GitHub, deploys
            to the Deployments API (production, or preview for pull requests). A
            failed post never fails a deploy or a run. The server can switch
            this off for every app with APP_GIT_STATUS_ENABLED=false.
          </InfoTip>
        </Label>
      </div>
      <div>
        <Button size="sm" onClick={submit} disabled={!dirty || save.isPending}>
          {save.isPending ? 'Saving...' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
