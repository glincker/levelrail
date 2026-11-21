import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  FIELD_RESOURCES,
  fieldDescriptorsQueryOptions,
} from '../../queries/fieldDescriptors'
import { DescriptorFields } from '../DescriptorFields'

type DnsProvider = 'cloudflare' | 'route53' | 'googlecloud'

interface DnsProviderFieldsProps {
  provider: DnsProvider
  values: Record<string, string>
  onChange: (key: string, value: string) => void
  idPrefix: string
  /** True once the secret fields have a stored value, so the inputs show a mask. */
  hasSecret: boolean
  fieldExtra?: (id: string) => ReactNode
}

const SECRET_MASK = '•'.repeat(12)

// The credential inputs of one DNS provider card, rendered from the shared field registry
// so a new provider field needs no card edit. Secrets stay blank and write only.
export function DnsProviderFields({
  provider,
  values,
  onChange,
  idPrefix,
  hasSecret,
  fieldExtra,
}: DnsProviderFieldsProps) {
  const { data } = useQuery({
    ...fieldDescriptorsQueryOptions(FIELD_RESOURCES.dnsProvider),
    retry: false,
  })
  const fields = (data?.fields ?? []).filter((f) => f.id !== 'provider')
  if (fields.length === 0) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          The form fields could not be loaded. Reload the page and try again.
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <DescriptorFields
      fields={fields}
      values={{ ...values, provider }}
      onChange={onChange}
      idPrefix={idPrefix}
      placeholder={(f) => (f.secret && hasSecret ? SECRET_MASK : undefined)}
      fieldExtra={(f) => fieldExtra?.(f.id)}
    />
  )
}
