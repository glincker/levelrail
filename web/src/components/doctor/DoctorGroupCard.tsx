import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { DoctorCheckRow } from '@/components/DoctorCheckRow'
import type { DoctorGroup } from '../../lib/doctorModel'

/** Above this many rows a group scrolls inside a virtualized window. */
export const DOCTOR_VIRTUALIZE_AFTER = 50
const ROW_ESTIMATE_PX = 72

function VirtualRows({
  group,
  recheck,
}: {
  group: DoctorGroup
  recheck: (checkId: string) => Promise<void>
}) {
  const parent = useRef<HTMLDivElement>(null)
  const virt = useVirtualizer({
    count: group.checks.length,
    getScrollElement: () => parent.current,
    estimateSize: () => ROW_ESTIMATE_PX,
    overscan: 6,
  })
  return (
    <div ref={parent} className="max-h-[32rem] overflow-y-auto">
      <div className="relative" style={{ height: virt.getTotalSize() }}>
        {virt.getVirtualItems().map((item) => {
          const check = group.checks[item.index]
          if (!check) return null
          return (
            <div
              key={check.code}
              ref={virt.measureElement}
              data-index={item.index}
              className="absolute inset-x-0 border-b border-border"
              style={{ transform: `translateY(${item.start}px)` }}
            >
              <DoctorCheckRow
                check={check}
                onRecheck={
                  check.check_id
                    ? () => recheck(check.check_id ?? '')
                    : undefined
                }
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}

/** DoctorGroupCard is one section of results: plain rows, or a virtualized window past 50. */
export function DoctorGroupCard({
  group,
  recheck,
}: {
  group: DoctorGroup
  recheck: (checkId: string) => Promise<void>
}) {
  const { t } = useTranslation('settings')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(`doctorPage.groups.${group.id}.title`)}</CardTitle>
        <CardDescription>
          {t(`doctorPage.groups.${group.id}.description`)}
        </CardDescription>
      </CardHeader>
      {group.checks.length > DOCTOR_VIRTUALIZE_AFTER ? (
        <CardContent>
          <VirtualRows group={group} recheck={recheck} />
        </CardContent>
      ) : (
        <CardContent className="divide-y divide-border">
          {group.checks.map((check) => (
            <DoctorCheckRow
              key={check.code}
              check={check}
              onRecheck={
                check.check_id ? () => recheck(check.check_id ?? '') : undefined
              }
            />
          ))}
        </CardContent>
      )}
    </Card>
  )
}
