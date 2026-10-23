import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  setEnvironmentScope,
  useEnvironmentScope,
} from '../../lib/environmentScope'
import { setProjectScope, useProjectScope } from '../../lib/projectScope'
import { globalEnvironmentsQueryOptions } from '../../queries/globalEnvironments'
import { projectListQueryOptions } from '../../queries/projects'

/** useScopeName returns the picked "Project / Environment" text, or '' when nothing is picked. */
export function useScopeName(): string {
  const { t } = useTranslation('dashboard')
  const environment = useEnvironmentScope()
  const project = useProjectScope()
  const scoped = environment !== '' || project !== ''
  const projects = useQuery({
    ...projectListQueryOptions(),
    retry: false,
    enabled: project !== '',
  })
  const environments = useQuery({
    ...globalEnvironmentsQueryOptions(),
    retry: false,
    enabled: environment !== '',
  })
  if (!scoped) return ''
  const projectName =
    project === ''
      ? t('scope.allProjects')
      : (projects.data?.find((p) => p.id === project)?.name ?? project)
  const envName =
    environment === ''
      ? ''
      : (environments.data?.find((e) => e.id === environment)?.name ??
        environment)
  return [projectName, envName].filter(Boolean).join(' / ')
}

export function clearScope(): void {
  setProjectScope('')
  setEnvironmentScope('')
}
