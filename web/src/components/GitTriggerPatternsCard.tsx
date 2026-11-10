import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FunnelGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../queries/gitSources'
import { useSetGitDeploySettings } from '../queries/gitDeploySettings'

// GitTriggerPatternsCard narrows which tags (release mode) or branches (push
// mode) deploy. The server stores one pattern per line and validates on save.
export function GitTriggerPatternsCard({ appName }: { appName: string }) {
  const source = useGitSource(appName)
  if (!source.data) {
    return null
  }
  const release = source.data.trigger_mode === 'release'
  const current = release
    ? (source.data.tag_pattern ?? '')
    : (source.data.branch_pattern ?? '')
  return (
    <TriggerPatternsForm
      key={`${source.data.trigger_mode}:${current}`}
      appName={appName}
      release={release}
      branch={source.data.branch}
      current={current}
    />
  )
}

function TriggerPatternsForm({
  appName,
  release,
  branch,
  current,
}: {
  appName: string
  release: boolean
  branch: string
  current: string
}) {
  const { t } = useTranslation('gitSource')
  const save = useSetGitDeploySettings(appName)
  const [value, setValue] = useState(current)
  const id = 'git-trigger-pattern'

  const submit = () =>
    save.mutate(release ? { tag_pattern: value } : { branch_pattern: value }, {
      onSuccess: () =>
        toast.add({ title: t('triggerPatterns.saved'), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: t('triggerPatterns.failed'),
          description: e.message,
          type: 'error',
        }),
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FunnelGlassIcon className="size-5 text-muted-foreground" />
          {t('triggerPatterns.title')}
        </CardTitle>
        <CardDescription>
          {release
            ? t('triggerPatterns.tagHelp')
            : t('triggerPatterns.branchHelp', { branch })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor={id}>
            {release
              ? t('triggerPatterns.tagLabel')
              : t('triggerPatterns.branchLabel')}
          </Label>
          <Textarea
            id={id}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            rows={3}
            spellCheck={false}
            className="font-mono text-xs"
            placeholder={release ? 'v*\n!v*-rc*' : 'main\nrelease/*'}
          />
          <p className="text-xs text-muted-foreground">
            {t('triggerPatterns.syntax')}
          </p>
        </div>
        <Button
          size="sm"
          onClick={submit}
          disabled={value === current || save.isPending}
        >
          {save.isPending
            ? t('triggerPatterns.saving')
            : t('triggerPatterns.save')}
        </Button>
      </CardContent>
    </Card>
  )
}
