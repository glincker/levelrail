import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useDeployServiceTemplateNow } from '../queries/serviceTemplates'
import type { ComposeDeployResponse } from '../queries/compose'

export interface DeployedTemplate {
  templateId: string
  response: ComposeDeployResponse
}

// Shared one-click deploy handler (POST /api/v1/service-templates/{id}/deploy).
// With showResult the caller renders TemplateDeployResult from `deployed`;
// otherwise it toasts and lands on the new app's page.
export function useDeployTemplateNow(
  onDeployed?: () => void,
  options: { showResult?: boolean } = {},
) {
  const { t } = useTranslation('templates')
  const deployNow = useDeployServiceTemplateNow()
  const navigate = useNavigate()
  const [deployed, setDeployed] = useState<DeployedTemplate | null>(null)

  function deploy(templateId: string) {
    deployNow.mutate(templateId, {
      onSuccess: (result) => {
        onDeployed?.()
        if (options.showResult) {
          setDeployed({ templateId, response: result })
          return
        }
        const landingName = result.services[0]?.name ?? result.app_id
        toast.add({
          title: t('result.toastDeployed', { name: result.app_id }),
          description: t('result.toastRunning', {
            count: result.services.length,
          }),
          type: 'success',
        })
        void navigate({ to: '/apps/$name', params: { name: landingName } })
      },
      onError: (error) => {
        toast.add({
          title: t('result.deployFailed'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return {
    deploy,
    deployed,
    clearDeployed: () => {
      setDeployed(null)
    },
    isPending: deployNow.isPending,
    pendingId: deployNow.isPending ? deployNow.variables : undefined,
    isError: deployNow.isError,
    error: deployNow.error,
  }
}
