import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { appDetailQueryOptions, useUpdateApp } from '../../queries/apps'
import { useListeningPorts } from '../../queries/domainWizard'
import { PortStepBody } from '../DomainWizardSteps'
import { StepBody } from './DomainStepperParts'

// Container port check inside the stepper, so the port can be fixed before the certificate step.
export function PortStep({
  appName,
  onNext,
}: {
  appName: string
  onNext: () => void
}) {
  const { t } = useTranslation('domains')
  const app = useQuery(appDetailQueryOptions(appName))
  const listening = useListeningPorts(appName, true)
  const update = useUpdateApp(appName)
  const detail = app.data
  return (
    <StepBody>
      {detail ? (
        <PortStepBody
          appPort={detail.port}
          data={listening.data}
          isFetching={listening.isFetching}
          switching={update.isPending}
          onSwitch={(port) => {
            update.mutate(
              { ...detail, port },
              {
                onSuccess: () => {
                  toast.add({ title: t('port.saved'), type: 'success' })
                  void listening.refetch()
                },
              },
            )
          }}
          onRecheck={() => {
            void listening.refetch()
          }}
        />
      ) : null}
      <Button type="button" size="sm" onClick={onNext}>
        <ArrowRightIcon />
        {t('stepper.port.next')}
      </Button>
    </StepBody>
  )
}
