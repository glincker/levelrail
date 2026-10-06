import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { EnvironmentKind } from '../../types/environment'

const KIND_VARIANT = {
  dev: 'muted',
  test: 'default',
  uat: 'warning',
  production: 'destructive',
  preview: 'outline',
  custom: 'outline',
} as const

export function EnvironmentKindBadge({ kind }: { kind: EnvironmentKind }) {
  const { t } = useTranslation('environments')
  return <Badge variant={KIND_VARIANT[kind]}>{t(`kind.${kind}`)}</Badge>
}
