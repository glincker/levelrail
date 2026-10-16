import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  DotsSixVerticalIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { BrandTile } from '../BrandLogoBadge'
import { aiProviderBrand } from '../../lib/providerBrands'
import type { AiProvider, AiSelection } from '../../queries/aiProviders'
import { moveItem } from './aiProviderState'

export function AiFallbackList({
  fallback,
  providers,
  canWrite,
  pending,
  onChange,
}: {
  fallback: AiSelection[]
  providers: AiProvider[]
  canWrite: boolean
  pending: boolean
  onChange: (next: AiSelection[]) => void
}) {
  const { t } = useTranslation('settings')
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  const disabled = !canWrite || pending

  if (fallback.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('aiAssistantPage.fallback.empty')}
      </p>
    )
  }

  return (
    <ol className="space-y-2" aria-label={t('aiAssistantPage.fallback.title')}>
      {fallback.map((f, i) => {
        const owner = providers.find((p) => p.id === f.provider)
        const label = owner?.label ?? f.provider
        return (
          <li
            key={`${f.provider}/${f.model}`}
            draggable={!disabled}
            onDragStart={() => {
              setDragFrom(i)
            }}
            onDragOver={(e) => {
              e.preventDefault()
            }}
            onDrop={() => {
              if (dragFrom !== null) onChange(moveItem(fallback, dragFrom, i))
              setDragFrom(null)
            }}
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-2 py-1.5"
          >
            <DotsSixVerticalIcon
              className="size-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
            <span className="w-5 text-xs text-muted-foreground tabular-nums">
              {i + 1}
            </span>
            <BrandTile
              name={aiProviderBrand(owner?.brand)}
              label={label}
              size="xs"
            />
            <span className="min-w-0 flex-1 truncate text-sm">
              {label}
              <span className="text-muted-foreground"> {f.model}</span>
            </span>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={disabled || i === 0}
              aria-label={t('aiAssistantPage.fallback.moveUp', { name: label })}
              onClick={() => {
                onChange(moveItem(fallback, i, i - 1))
              }}
            >
              <ArrowUpIcon />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={disabled || i === fallback.length - 1}
              aria-label={t('aiAssistantPage.fallback.moveDown', {
                name: label,
              })}
              onClick={() => {
                onChange(moveItem(fallback, i, i + 1))
              }}
            >
              <ArrowDownIcon />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={disabled}
              aria-label={t('aiAssistantPage.fallback.remove', { name: label })}
              onClick={() => {
                onChange(fallback.filter((_, j) => j !== i))
              }}
            >
              <XIcon />
            </Button>
          </li>
        )
      })}
    </ol>
  )
}
