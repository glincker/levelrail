import { TONE } from '@/components/kit/tone'
import { statusLook } from '../../lib/deliveryStatus'

// The label is rendered by the caller so the colour is never the only signal.
export function StatusIcon({
  status,
  className = 'size-4',
}: {
  status: string
  className?: string
}) {
  const look = statusLook(status)
  const Icon = look.icon
  return (
    <Icon
      aria-hidden="true"
      className={`${className} ${TONE[look.tone].text} ${look.spin ? 'animate-spin motion-reduce:animate-none' : ''}`}
    />
  )
}
