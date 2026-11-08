import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { appGroupQueryOptions } from '../queries/appGroup'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// Services of a group are separate apps, so picking one opens that app's logs.
// Non-suspending: the Logs page must not fail because this control did.
export function LogsServiceSelect({ name }: { name: string }) {
  const { t } = useTranslation('observability')
  const navigate = useNavigate()
  const { data } = useQuery(appGroupQueryOptions(name))
  const services = data?.services ?? []
  if (services.length < 2) return null

  return (
    <div className="flex items-center gap-2">
      <span id="logs-service-label" className="text-sm text-muted-foreground">
        {t('logs.serviceFilter.label')}
      </span>
      <Select<string>
        value={name}
        onValueChange={(next) => {
          if (!next || next === name) return
          void navigate({
            to: '/apps/$name/logs',
            params: { name: next },
            search: (prev: Record<string, unknown>) => prev,
          })
        }}
      >
        <SelectTrigger
          aria-labelledby="logs-service-label"
          className="h-8 w-auto min-w-40"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {services.map((svc) => (
            <SelectItem key={svc.name} value={svc.name}>
              {svc.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
