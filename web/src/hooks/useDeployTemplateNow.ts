import { useNavigate } from '@tanstack/react-router'
import { toast } from '@/components/ui/toast'
import { useDeployServiceTemplateNow } from '../queries/serviceTemplates'

// Shared one-click deploy handler for a template with no required
// configuration (POST /api/v1/service-templates/{id}/deploy): toasts the
// result and lands on the new app's own page. Used by the wizard's
// BrowseTemplatesFields grid, the standalone /templates catalog grid, and
// the /templates/$id detail page's "Deploy now" button, so this
// toast/navigate glue lives in exactly one place around the shared
// useDeployServiceTemplateNow mutation.
export function useDeployTemplateNow(onDeployed?: () => void) {
  const deployNow = useDeployServiceTemplateNow()
  const navigate = useNavigate()

  function deploy(templateId: string) {
    deployNow.mutate(templateId, {
      onSuccess: (result) => {
        onDeployed?.()
        const landingName = result.services[0]?.name ?? result.app_id
        toast.add({
          title: `"${result.app_id}" deployed`,
          description: `${result.services.length} ${
            result.services.length === 1 ? 'service' : 'services'
          } running now.`,
          type: 'success',
        })
        void navigate({ to: '/apps/$name', params: { name: landingName } })
      },
      onError: (error) => {
        toast.add({
          title: 'Deploy failed',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return {
    deploy,
    isPending: deployNow.isPending,
    pendingId: deployNow.isPending ? deployNow.variables : undefined,
    isError: deployNow.isError,
    error: deployNow.error,
  }
}
