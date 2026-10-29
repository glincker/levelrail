import { useState } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontalGlassIcon } from '@thesvg/ui-react'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { RestartNeeded } from '@/components/settings/BetaFeatureCard'
import { useIsRoot } from '@/hooks/useIsRoot'
import { cn } from '@/lib/utils'
import {
  useServerProfile,
  useSetServerProfile,
  type ServerProfileId,
  type ServerProfileInfo,
  type ServerProfileProblem,
} from '@/queries/serverProfile'

function includes(
  p: ServerProfileInfo,
  t: TFunction<'betaFeatures'>,
): string[] {
  return [
    t(p.mesh ? 'profile.facts.meshOn' : 'profile.facts.meshOff'),
    t(p.plugins ? 'profile.facts.pluginsOn' : 'profile.facts.pluginsOff'),
    t(
      p.experimental_features > 0
        ? 'profile.facts.experimentalOn'
        : 'profile.facts.experimentalOff',
    ),
    t('profile.facts.retention', { days: p.retention_days }),
    ...(p.loopback_agent ? [t('profile.facts.loopbackAgent')] : []),
    ...(p.lazy_loops ? [t('profile.facts.lazyLoops')] : []),
    ...(p.posture_recommendations ? [t('profile.facts.recommendations')] : []),
    ...(p.sbom_scan ? [t('profile.facts.sbom')] : []),
  ]
}

export function ServerProfileCard() {
  const { t } = useTranslation('betaFeatures')
  const isRoot = useIsRoot()
  const { data } = useServerProfile()
  const setProfile = useSetServerProfile()
  const [problems, setProblems] = useState<ServerProfileProblem[]>([])
  const [needsConfirm, setNeedsConfirm] = useState<ServerProfileId | null>(null)

  if (!data) return null
  const blocked = problems.some((p) => p.blocking)

  function choose(id: ServerProfileId, confirm = false) {
    setProfile.mutate(
      { profile: id, confirm },
      {
        onSuccess: (res) => {
          if (res.saved) {
            setProblems([])
            setNeedsConfirm(null)
            toast.add({
              type: 'success',
              title: t('profile.saved', { name: t(`profile.names.${id}`) }),
            })
            return
          }
          setProblems(res.profile.problems)
          setNeedsConfirm(
            res.profile.problems.some((p) => p.blocking) ? null : id,
          )
        },
        onError: (err) =>
          toast.add({
            type: 'error',
            title: t('profile.error', { message: err.message }),
          }),
      },
    )
  }

  return (
    <Card id="server-profile">
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <SlidersHorizontalGlassIcon aria-hidden="true" className="size-5.5" />
          {t('profile.title')}
          <Badge variant="success">
            {t(`profile.names.${data.configured}`)}
          </Badge>
          {data.pinned_by_env ? (
            <Badge variant="outline">{t('profile.pinned')}</Badge>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('profile.description')}
        </p>
        {data.recommended && data.recommended !== data.configured ? (
          <Alert>
            {t('profile.recommended', {
              name: t(`profile.names.${data.recommended}`),
            })}
          </Alert>
        ) : null}
        {data.pinned_by_env ? (
          <p className="text-xs text-muted-foreground">
            {t('profile.pinnedHelp', { env: data.env_var })}
          </p>
        ) : null}
        <ul className="grid gap-3 md:grid-cols-3">
          {data.profiles.map((p) => {
            const selected = p.id === data.configured
            return (
              <li
                key={p.id}
                className={cn(
                  'space-y-2 rounded-md border p-3',
                  selected && 'border-primary',
                )}
              >
                <p className="text-sm font-medium">
                  {t(`profile.names.${p.id}`)}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t(`profile.summary.${p.id}`)}
                </p>
                <ul className="list-disc space-y-1 pl-4 text-xs text-muted-foreground">
                  {includes(p, t).map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
                {isRoot && !data.pinned_by_env ? (
                  <Button
                    type="button"
                    size="sm"
                    variant={selected ? 'outline' : 'default'}
                    disabled={selected || setProfile.isPending}
                    onClick={() => {
                      choose(p.id)
                    }}
                  >
                    {selected ? t('profile.current') : t('profile.use')}
                  </Button>
                ) : null}
              </li>
            )
          })}
        </ul>
        {problems.length > 0 ? (
          <Alert variant={blocked ? 'destructive' : 'default'}>
            <ul className="space-y-1">
              {problems.map((p) => (
                <li key={p.code}>{p.message}</li>
              ))}
            </ul>
            {needsConfirm ? (
              <Button
                type="button"
                size="sm"
                className="mt-2"
                disabled={setProfile.isPending}
                onClick={() => {
                  choose(needsConfirm, true)
                }}
              >
                {t('profile.confirm')}
              </Button>
            ) : null}
          </Alert>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t('profile.neverDeletes')}
        </p>
        {!isRoot ? (
          <p className="text-xs text-muted-foreground">
            {t('profile.readOnly')}
          </p>
        ) : null}
        {data.restart_required ? <RestartNeeded /> : null}
      </CardContent>
    </Card>
  )
}
