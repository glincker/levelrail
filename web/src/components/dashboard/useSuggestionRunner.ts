import { useNavigate } from '@tanstack/react-router'
import { usePaletteAppActions } from '../../hooks/usePaletteAppActions'
import type { ActionSpec } from '../../lib/fleetSuggestions'

export function useSuggestionRunner(): (spec: ActionSpec) => void {
  const navigate = useNavigate()
  const { restartApp, redeployApp } = usePaletteAppActions()
  return (spec) => {
    switch (spec.kind) {
      case 'restart':
        restartApp(spec.app)
        return
      case 'redeploy':
        redeployApp(spec.app, spec.image)
        return
      case 'logs':
        void navigate({ to: '/apps/$name/logs', params: { name: spec.app } })
        return
      case 'deploys':
        void navigate({ to: '/apps/$name/deploys', params: { name: spec.app } })
        return
      case 'node':
        void navigate({ to: '/nodes/$id', params: { id: spec.id } })
        return
      case 'domains':
        void navigate({ to: '/domains' })
        return
      case 'cleanup':
        void navigate({ to: '/settings/general' })
        return
      case 'system':
        void navigate({ to: '/settings/system-status' })
        return
    }
  }
}
