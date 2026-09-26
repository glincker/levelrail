import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import type { ImportService } from '../queries/imports'

/**
 * One service's discovered env variables with required and secret
 * indicators and an inline value input. Secret-looking defaults are never
 * sent by the server, so a secret row always starts empty.
 */
export function ImportEnvTable({
  service,
  values,
  onChange,
}: {
  service: ImportService
  values: Record<string, string>
  onChange: (key: string, value: string) => void
}) {
  const env = service.env ?? []
  if (env.length === 0) return null
  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="text-left text-xs text-muted-foreground">
          <th className="py-1 pr-2 font-medium">Variable</th>
          <th className="py-1 pr-2 font-medium">Value</th>
        </tr>
      </thead>
      <tbody>
        {env.map((e) => (
          <tr key={e.key} className="align-top">
            <td className="py-1 pr-2">
              <div className="font-mono text-xs">{e.key}</div>
              <div className="mt-0.5 flex gap-1">
                {e.required ? (
                  <Badge variant="destructive">Required</Badge>
                ) : null}
                {e.secret ? <Badge variant="warning">Secret</Badge> : null}
                {!e.required && e.has_default ? (
                  <Badge variant="muted">Has default</Badge>
                ) : null}
              </div>
            </td>
            <td className="py-1">
              <Input
                type={e.secret ? 'password' : 'text'}
                autoComplete="off"
                aria-label={`Value for ${e.key}`}
                value={values[e.key] ?? ''}
                placeholder={e.value ?? (e.has_default ? 'default' : '')}
                onChange={(ev) => {
                  onChange(e.key, ev.target.value)
                }}
              />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
