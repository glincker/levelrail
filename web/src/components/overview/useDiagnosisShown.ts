import { useFailureContext } from '../../queries/investigate'
import { useStuck } from '../../queries/stuck'

/** True when the app is failing in a way the Diagnosis card explains. */
export function useDiagnosisShown(appName: string): boolean {
  const stuck = useStuck('app', appName).data
  const failure = useFailureContext(appName).data
  return Boolean(
    (failure && failure.state !== 'healthy') || (stuck && !stuck.ready),
  )
}
