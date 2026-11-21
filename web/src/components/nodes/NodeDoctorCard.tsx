import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useNodeDoctor, type NodeDoctorCheck } from '../../queries/nodeInsight'

const VARIANT: Record<
  NodeDoctorCheck['status'],
  'success' | 'warning' | 'destructive' | 'muted'
> = { pass: 'success', warn: 'warning', fail: 'destructive', unknown: 'muted' }

// Doctor results stay behind one disclosure so a healthy node adds no noise.
export function NodeDoctorCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const { data, isPending, isError } = useNodeDoctor(nodeId, open)

  return (
    <Card id="node-doctor">
      <CardHeader>
        <CardTitle>{t('doctor.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger className="text-sm font-medium">
            {open ? t('doctor.hide') : t('doctor.run')}
          </CollapsibleTrigger>
          <CollapsiblePanel className="space-y-3 pt-3">
            {isPending ? (
              <p role="status" className="text-sm text-muted-foreground">
                {t('doctor.running')}
              </p>
            ) : isError ? (
              <p role="alert" className="text-sm text-muted-foreground">
                {t('doctor.unavailable')}
              </p>
            ) : (
              <ul className="space-y-2">
                {data.checks.map((c) => (
                  <li key={c.id} className="space-y-1 text-sm">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant={VARIANT[c.status]}>
                        {t(`doctor.status.${c.status}`)}
                      </Badge>
                      <span className="font-medium">
                        {t(`doctor.checks.${c.id}`, { defaultValue: c.id })}
                      </span>
                      <span className="text-muted-foreground">{c.detail}</span>
                    </div>
                    {c.fix ? (
                      <p className="pl-1 text-xs text-muted-foreground">
                        {c.fix}
                        {c.fix_command ? (
                          <code className="ml-2 font-mono">
                            {c.fix_command}
                          </code>
                        ) : null}
                      </p>
                    ) : null}
                  </li>
                ))}
                {!data.reported ? (
                  <li className="text-xs text-muted-foreground">
                    {t('doctor.noReport')}
                  </li>
                ) : null}
              </ul>
            )}
          </CollapsiblePanel>
        </Collapsible>
      </CardContent>
    </Card>
  )
}
