import { RocketLaunchIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useRedeployApp } from '../hooks/useRedeployApp'

export function RedeployAppButton({
  name,
  image,
}: {
  name: string
  image: string
}) {
  const { redeploy, isPending } = useRedeployApp(name, image)
  return (
    <Button variant="outline" size="sm" disabled={isPending} onClick={redeploy}>
      <RocketLaunchIcon className="size-3.5" aria-hidden="true" />
      {isPending ? 'Redeploying...' : 'Redeploy'}
    </Button>
  )
}
