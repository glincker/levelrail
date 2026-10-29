import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ApiError } from '../lib/apiError'

const HINT_CODES = [
  'published_port_invalid',
  'published_port_conflict',
  'published_port_remote_node',
  'published_port_in_use',
] as const

// Server message first (it names the port and service), then a one-line fix hint
// for the three published-port codes. Other errors show the message alone.
export function PublishedPortError({ error }: { error: Error }) {
  const { t } = useTranslation('compose')
  const code = error instanceof ApiError ? error.code : undefined
  const hintCode = HINT_CODES.find((c) => c === code)
  return (
    <Alert variant="destructive">
      <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
      <AlertDescription>
        <span>{error.message}</span>
        {hintCode ? (
          <span className="mt-1 block text-xs opacity-80">
            {t(`publishedPortError.${hintCode}`)}
          </span>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
