import { useState } from 'react'
import { SlidersHorizontalIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { InfoTip, SkeletonLine } from './kit'
import { useGitSource } from '../queries/gitSources'
import { usePreviewPolicy, useSetPreviewPolicy } from '../queries/previewPolicy'
import type { PreviewOnLimit, PreviewPolicy } from '../types/previewEnvironment'

const ON_LIMIT_LABEL: Record<PreviewOnLimit, string> = {
  evict_oldest: 'Remove the oldest preview',
  reject: 'Skip the new pull request',
}

function capText(cap: number): string {
  return cap > 0 ? String(cap) : 'no limit'
}

function UsageLine({
  label,
  live,
  cap,
}: {
  label: string
  live: number
  cap: number
}) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium text-foreground">
        {live} live of {capText(cap)}
      </dd>
    </div>
  )
}

// PreviewPolicyCard edits the per-app preview policy (GET/PUT
// /api/v1/apps/{name}/preview-policy): what happens at the concurrency cap,
// whether fork pull requests may deploy, and how long a preview lives.
export function PreviewPolicyCard({ appName }: { appName: string }) {
  const gitSource = useGitSource(appName)
  const policy = usePreviewPolicy(appName)

  if (!gitSource.data) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SlidersHorizontalIcon className="size-4 text-muted-foreground" />
          Preview limits and safety
          <InfoTip label="About preview limits">
            Caps come from the control plane settings
            APP_PREVIEW_ENV_MAX_PER_APP and APP_PREVIEW_ENV_MAX_TOTAL. The
            choices below decide what happens when a cap is full, whether pull
            requests from forks may deploy, and how long an idle preview lives.
          </InfoTip>
        </CardTitle>
        <CardDescription>
          Keep previews from piling up, and decide whether code from forks may
          run.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {policy.isPending ? (
          <div className="space-y-2" aria-label="Loading preview policy">
            <SkeletonLine width="60%" />
            <SkeletonLine width="40%" />
          </div>
        ) : policy.isError ? (
          <p className="text-sm text-muted-foreground">
            Preview policy is not available on this control plane.
          </p>
        ) : (
          <PreviewPolicyForm
            key={`${policy.data.on_limit}:${policy.data.ttl_hours}`}
            appName={appName}
            policy={policy.data}
          />
        )}
      </CardContent>
    </Card>
  )
}

function PreviewPolicyForm({
  appName,
  policy,
}: {
  appName: string
  policy: PreviewPolicy
}) {
  const setPolicy = useSetPreviewPolicy(appName)
  const [ttl, setTtl] = useState(String(policy.ttl_hours))
  const ttlValue = Number(ttl)
  const ttlValid = Number.isInteger(ttlValue) && ttlValue >= 0
  const ttlDirty = ttlValue !== policy.ttl_hours

  function save(update: Parameters<typeof setPolicy.mutate>[0], title: string) {
    setPolicy.mutate(update, {
      onSuccess: () => toast.add({ title, type: 'success' }),
      onError: (error) =>
        toast.add({
          title: 'Could not update preview policy.',
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="space-y-5">
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
        <UsageLine
          label="This app"
          live={policy.live_count}
          cap={policy.max_per_app}
        />
        <UsageLine
          label="All apps"
          live={policy.live_total}
          cap={policy.max_total}
        />
      </dl>

      <div className="space-y-1.5">
        <Label htmlFor="preview-on-limit" className="flex items-center gap-1.5">
          When the limit is full
          <InfoTip label="About the limit policy">
            Removing the oldest preview keeps the newest pull requests live.
            Skipping the new pull request leaves what is running alone and shows
            why on the pull request.
          </InfoTip>
        </Label>
        <Select
          value={policy.on_limit}
          onValueChange={(value) => {
            if (value === 'evict_oldest' || value === 'reject') {
              save({ on_limit: value }, 'Limit policy saved.')
            }
          }}
        >
          <SelectTrigger
            id="preview-on-limit"
            className="w-full sm:w-72"
            disabled={setPolicy.isPending}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(Object.keys(ON_LIMIT_LABEL) as PreviewOnLimit[]).map((key) => (
              <SelectItem key={key} value={key}>
                {ON_LIMIT_LABEL[key]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="flex items-center gap-1.5 text-sm font-medium text-foreground">
              Deploy previews for pull requests from forks
              <InfoTip label="About fork previews">
                A fork can change the code that runs. A preview gets this
                app&apos;s environment variables and secrets, so by default a
                fork waits until someone approves it.
              </InfoTip>
            </p>
            <p className="text-sm text-muted-foreground">
              Off by default. When off, fork pull requests wait for you to
              approve them.
            </p>
          </div>
          <Switch
            checked={policy.allow_fork_previews}
            disabled={setPolicy.isPending}
            onCheckedChange={(next) => {
              save(
                { allow_fork_previews: next },
                next
                  ? 'Fork previews allowed.'
                  : 'Fork previews need approval.',
              )
            }}
            aria-label="Deploy previews for pull requests from forks"
          />
        </div>
        {policy.allow_fork_previews ? (
          <Alert variant="destructive">
            <AlertDescription>
              Anyone who can open a pull request from a fork can run code with
              this app&apos;s environment variables and secrets. Turn this off
              unless every contributor is trusted.
            </AlertDescription>
          </Alert>
        ) : null}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="preview-ttl" className="flex items-center gap-1.5">
          Remove idle previews after (hours)
          <InfoTip label="About the preview lifetime">
            A preview with no new commits for this long is removed, in case the
            pull request close event never arrived. 0 uses the platform default
            of {policy.effective_ttl_hours} hours.
          </InfoTip>
        </Label>
        <div className="flex items-center gap-2">
          <Input
            id="preview-ttl"
            inputMode="numeric"
            className="w-28"
            value={ttl}
            onChange={(e) => {
              setTtl(e.target.value)
            }}
            aria-invalid={!ttlValid}
          />
          <Button
            size="sm"
            disabled={!ttlDirty || !ttlValid || setPolicy.isPending}
            onClick={() => {
              save({ ttl_hours: ttlValue }, 'Preview lifetime saved.')
            }}
          >
            Save
          </Button>
        </div>
      </div>
    </div>
  )
}
