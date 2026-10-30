import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CheckIcon,
  FolderSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { isFeatureVisible } from '../../lib/experimental'
import {
  activateEnvironmentScope,
  setEnvironmentScope,
  useEnvironmentScope,
} from '../../lib/environmentScope'
import { defaultEnvironmentFor, environmentsFor } from '../../lib/scopeChoices'
import { setProjectScope, useProjectScope } from '../../lib/projectScope'
import { useGlobalEnvironments } from '../../queries/globalEnvironments'
import { projectListQueryOptions } from '../../queries/projects'

function Row({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={active}
      onClick={onClick}
      className={cn(
        'flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted',
        active && 'bg-muted font-medium',
      )}
    >
      <span className="truncate">{children}</span>
      {active ? <CheckIcon aria-hidden="true" className="size-3.5" /> : null}
    </button>
  )
}

/** ScopePicker is the top bar's Project > Environment choice; single-project, single-environment setups never see it. */
export function ScopePicker() {
  const { t } = useTranslation('environments')
  const [open, setOpen] = useState(false)
  const enabled = isFeatureVisible(
    'global-environments',
    useExperimentalFeatures(),
  )
  const envScope = useEnvironmentScope()
  const projectScope = useProjectScope()
  const projects = useQuery({ ...projectListQueryOptions(), retry: false }).data
  const environments = useGlobalEnvironments(enabled).data

  useEffect(() => {
    activateEnvironmentScope(enabled)
  }, [enabled])

  const projectList = projects ?? []
  const selectable = (environments ?? []).filter((e) => e.kind !== 'preview')
  const showEnvs = enabled && selectable.length > 1
  if (projectList.length < 2 && !showEnvs) return null

  const envChoices = environmentsFor(selectable, projectScope)
  const project = projectList.find((p) => p.id === projectScope)
  const env = selectable.find((e) => e.id === envScope)
  const label = [project?.name ?? t('picker.allProjects'), env?.name]
    .filter(Boolean)
    .join(' / ')

  function pickProject(id: string) {
    setProjectScope(id)
    if (!showEnvs) return
    const keep = environmentsFor(selectable, id).some((e) => e.id === envScope)
    if (!keep)
      setEnvironmentScope(
        id === '' ? '' : defaultEnvironmentFor(selectable, id),
      )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            className="w-full justify-start @4xl:w-auto"
            aria-label={t('picker.aria')}
          />
        }
      >
        <FolderSimpleIcon aria-hidden="true" />
        <span className="max-w-48 truncate">{label}</span>
        <CaretDownIcon aria-hidden="true" className="ml-auto size-3" />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[min(32rem,90vw)] p-2">
        <div className={cn('grid gap-2', showEnvs && 'sm:grid-cols-2')}>
          <div
            role="listbox"
            aria-label={t('picker.projects')}
            className="space-y-0.5"
          >
            <p className="px-2 pb-1 text-xs font-medium text-muted-foreground">
              {t('picker.projects')}
            </p>
            <Row
              active={projectScope === ''}
              onClick={() => {
                pickProject('')
              }}
            >
              {t('picker.allProjects')}
            </Row>
            {projectList.map((p) => (
              <Row
                key={p.id}
                active={projectScope === p.id}
                onClick={() => {
                  pickProject(p.id)
                }}
              >
                {p.name}
              </Row>
            ))}
          </div>
          {showEnvs ? (
            <div
              role="listbox"
              aria-label={t('picker.environments')}
              className="space-y-0.5"
            >
              <p className="px-2 pb-1 text-xs font-medium text-muted-foreground">
                {t('picker.environments')}
              </p>
              <Row
                active={envScope === ''}
                onClick={() => {
                  setEnvironmentScope('')
                  setOpen(false)
                }}
              >
                {t('picker.allEnvironments')}
              </Row>
              {envChoices.map((e) => (
                <Row
                  key={e.id}
                  active={envScope === e.id}
                  onClick={() => {
                    setEnvironmentScope(e.id)
                    setOpen(false)
                  }}
                >
                  {e.name}
                </Row>
              ))}
            </div>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  )
}
