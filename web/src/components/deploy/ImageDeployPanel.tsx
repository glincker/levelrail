import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, GitBranchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { DeploySafetyOptions } from '../DeploySafetyOptions'
import { EMPTY_DEPLOY_SAFETY } from '../../lib/imageDigest'
import { useFreezeBlocksDeploy } from '../../queries/deployFreeze'
import { useProtectedEnvironment } from '../../queries/environments'
import { useImageTagsOptional } from '../../queries/images'
import type { AppDetail } from '../../types/appDetail'
import { toast } from '@/components/ui/toast'
import {
  defaultScheduleWhen,
  validateScheduleAt,
  type ScheduleAtError,
} from '../../lib/scheduleAt'
import { useCreateScheduledDeploy } from '../../queries/scheduledDeploys'
import {
  ScheduleDeployField,
  type ScheduleDeployValue,
} from './ScheduleDeployField'
import { PendingChangesCard } from './PendingChangesCard'
import { ProtectedDeployNotice } from './ProtectedDeployNotice'
import { useDeployActions } from './useDeployActions'

const MAX_TAG_SUGGESTIONS = 6

// Replaces the tag of the current image so a suggested tag keeps its repository.
function withTag(current: string, tag: string): string {
  if (tag.includes('/') || tag.includes(':')) return tag
  const base = current.split('@')[0] ?? current
  const slash = base.lastIndexOf('/')
  const colon = base.lastIndexOf(':')
  const repo = colon > slash ? base.slice(0, colon) : base
  return `${repo}:${tag}`
}

export function ImageDeployPanel({
  app,
  onClose,
  onConnectRepo,
}: {
  app: AppDetail
  onClose: () => void
  onConnectRepo: () => void
}) {
  const { t } = useTranslation('smartDeploy')
  const current = app.image.split('@')[0] ?? app.image
  const [image, setImage] = useState(current)
  const [error, setError] = useState('')
  const [safety, setSafety] = useState(EMPTY_DEPLOY_SAFETY)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [ack, setAck] = useState(false)
  const tags = useImageTagsOptional(app.name)
  const protectedEnv = useProtectedEnvironment(app)
  const frozenWithoutReason = useFreezeBlocksDeploy(app.name, safety)
  const actions = useDeployActions({ appName: app.name, onDone: onClose })
  const createScheduled = useCreateScheduledDeploy(app.name)
  const [schedule, setSchedule] = useState<ScheduleDeployValue>(() => ({
    enabled: false,
    when: defaultScheduleWhen(Date.now()),
  }))
  const [scheduleError, setScheduleError] = useState<ScheduleAtError | null>(
    null,
  )
  const suggestions = (tags.data ?? []).slice(0, MAX_TAG_SUGGESTIONS)
  const scheduling = schedule.enabled
  const needsAck = protectedEnv?.protected === true && !ack && !scheduling

  const submit = () => {
    const value = image.trim()
    if (!value) {
      setError(t('image.required'))
      return
    }
    setError('')
    if (scheduling) {
      const { error: atError, iso } = validateScheduleAt(
        schedule.when,
        Date.now(),
      )
      setScheduleError(atError)
      if (atError) return
      createScheduled.mutate(
        { run_at: iso, image: value },
        {
          onSuccess: () => {
            toast.add({ title: t('schedule.toast.created'), type: 'success' })
            onClose()
          },
          onError: (e) => {
            toast.add({
              title: t('schedule.toast.failed'),
              description: e.message,
              type: 'error',
            })
          },
        },
      )
      return
    }
    actions.deployImage(value, { confirm: ack, pull: safety.pull })
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <div className="space-y-1.5">
        <label htmlFor="smart-deploy-image" className="text-sm font-medium">
          {t('image.imageLabel')}
        </label>
        <Input
          id="smart-deploy-image"
          className="font-mono"
          value={image}
          placeholder={t('image.placeholder')}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={error !== ''}
          onChange={(e) => {
            setImage(e.target.value)
          }}
        />
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
      </div>

      {suggestions.length > 0 ? (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">
            {t('image.suggestionsLabel')}
          </p>
          <div className="flex flex-wrap gap-1.5">
            {suggestions.map((s) => (
              <Button
                key={s.tag}
                type="button"
                size="sm"
                variant="outline"
                className="font-mono"
                onClick={() => {
                  setImage(withTag(current, s.tag))
                }}
              >
                {s.tag.split(':').pop()}
              </Button>
            ))}
          </div>
        </div>
      ) : null}

      <PendingChangesCard
        appName={app.name}
        image={image.trim() === current ? '' : image}
      />

      <Collapsible
        open={advancedOpen || frozenWithoutReason}
        onOpenChange={setAdvancedOpen}
      >
        <CollapsibleTrigger className="flex items-center gap-1 text-sm text-muted-foreground">
          <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
          {t('image.advanced')}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <DeploySafetyOptions
            appName={app.name}
            values={safety}
            onChange={setSafety}
          />
        </CollapsiblePanel>
      </Collapsible>

      <ScheduleDeployField
        value={schedule}
        error={scheduleError}
        onChange={(next) => {
          setSchedule(next)
          setScheduleError(null)
        }}
      />

      {protectedEnv?.protected && !scheduling ? (
        <ProtectedDeployNotice
          environmentName={protectedEnv.name}
          acknowledged={ack}
          onAcknowledgedChange={setAck}
        />
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={
          actions.pending ||
          createScheduled.isPending ||
          needsAck ||
          (frozenWithoutReason && !scheduling)
        }
      >
        {scheduling
          ? createScheduled.isPending
            ? t('schedule.submitting')
            : t('schedule.submit')
          : actions.imagePending
            ? t('image.deploying')
            : t('image.deploy')}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="w-full"
        onClick={onConnectRepo}
      >
        <GitBranchGlassIcon size="1.4em" aria-hidden="true" />
        {t('image.connectInstead')}
      </Button>
    </form>
  )
}
