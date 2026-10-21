import { useApp } from '../../queries/apps'
import { EnvironmentEditor } from '../env/EnvironmentEditor'
import { SecretRotationSection } from '../env/SecretRotationSection'

export function FunctionEnvironmentTab({ name }: Readonly<{ name: string }>) {
  const { data: app } = useApp(name)
  return (
    <div className="space-y-6">
      <EnvironmentEditor app={app} />
      <SecretRotationSection appName={app.name} />
    </div>
  )
}
