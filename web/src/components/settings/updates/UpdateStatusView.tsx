import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowUpGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ClockGlassIcon,
  ExternalLinkGlassIcon,
  InfoGlassIcon,
  LoaderCircleGlassIcon,
  RotateCwGlassIcon,
} from '@thesvg/ui-react'
import { StatusPill } from '@/components/kit'
import { TONE, type Tone } from '@/components/kit/tone'
import { Card, CardContent } from '@/components/ui/card'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { UpdateState } from './updateState'
import { UpdateBrandBanner } from './UpdateBrandBanner'

const STATE_TONE: Record<UpdateState, Tone> = {
  upToDate: 'success',
  available: 'info',
  scheduled: 'info',
  inProgress: 'info',
  checkFailed: 'danger',
  noReleases: 'neutral',
  hostBinary: 'neutral',
}

function StateIcon({ state }: { state: UpdateState }) {
  const cls = 'size-5'
  switch (state) {
    case 'upToDate':
      return <CircleCheckGlassIcon size="1.2em" className={cls} />
    case 'available':
      return <ArrowUpGlassIcon size="1.2em" className={cls} />
    case 'scheduled':
      return <ClockGlassIcon size="1.2em" className={cls} />
    case 'inProgress':
      return (
        <LoaderCircleGlassIcon
          size="1.2em"
          className={cn(cls, 'animate-spin')}
        />
      )
    case 'checkFailed':
      return <CircleAlertGlassIcon size="1.2em" className={cls} />
    default:
      return <InfoGlassIcon size="1.2em" className={cls} />
  }
}

export interface UpdateStatusViewProps {
  state: UpdateState
  currentVersion: string
  latestVersion: string | null
  edge: boolean
  lastChecked: string | null
  checking: boolean
  errorMessage?: string
  releaseUrl: string | null
  onCheck: () => void
  children?: ReactNode
}

// UpdateStatusView is presentational: the state is derived by the caller so
// every state can be rendered and tested without any query.
export function UpdateStatusView({
  state,
  currentVersion,
  latestVersion,
  edge,
  lastChecked,
  checking,
  errorMessage,
  releaseUrl,
  onCheck,
  children,
}: UpdateStatusViewProps) {
  const { t } = useTranslation('updates')
  const tone = TONE[STATE_TONE[state]]

  let headline: string
  switch (state) {
    case 'upToDate':
      headline = edge ? t('version.upToDateEdge') : t('version.upToDate')
      break
    case 'available':
      headline = t(edge ? 'version.latestEdge' : 'version.latest', {
        version: latestVersion ?? '',
      })
      break
    case 'scheduled':
      headline = t('status.state.scheduled')
      break
    case 'inProgress':
      headline = t('status.inProgressBody')
      break
    case 'checkFailed':
      headline = t('status.checkFailedTitle')
      break
    case 'hostBinary':
      headline = t('version.edgeHostBinary')
      break
    default:
      headline = t('version.none')
  }

  return (
    <Card>
      <CardContent className="space-y-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 flex-1 space-y-2">
            <UpdateBrandBanner
              version={currentVersion}
              stateClassName={cn(tone.soft, tone.text)}
              stateIcon={<StateIcon state={state} />}
              statePill={
                <StatusPill
                  tone={STATE_TONE[state]}
                  size="sm"
                  label={t(`status.state.${state}`)}
                />
              }
            />
            <div className="min-w-0 space-y-2">
              <h2 className="text-base font-medium text-pretty">{headline}</h2>
              {state === 'checkFailed' ? (
                <p
                  role="alert"
                  className="max-w-prose text-sm text-muted-foreground"
                >
                  {t('status.checkFailedBody', {
                    message: errorMessage ?? '',
                  })}
                </p>
              ) : null}
              <p className="text-xs text-muted-foreground">
                {lastChecked
                  ? t('status.lastChecked', { time: lastChecked })
                  : t('status.neverChecked')}
              </p>
            </div>
          </div>
          <div className="flex flex-col gap-2 sm:shrink-0 sm:flex-row sm:items-center">
            <Button
              type="button"
              variant="ghost"
              disabled={checking}
              className="w-full sm:w-auto"
              onClick={onCheck}
            >
              <RotateCwGlassIcon
                size="1.2em"
                className={checking ? 'animate-spin' : ''}
              />
              {checking ? t('status.checking') : t('status.checkNow')}
            </Button>
            {!edge && state === 'available' && releaseUrl ? (
              <a
                href={releaseUrl}
                target="_blank"
                rel="noopener noreferrer"
                className={cn(
                  buttonVariants({ variant: 'outline' }),
                  'w-full sm:w-auto',
                )}
              >
                {t('status.viewNotes')}
                <ExternalLinkGlassIcon size="1.2em" />
              </a>
            ) : null}
          </div>
        </div>
        {children}
      </CardContent>
    </Card>
  )
}
