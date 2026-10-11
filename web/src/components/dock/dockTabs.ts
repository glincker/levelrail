import type { ComponentType } from 'react'
import { ChatCircleTextIcon } from '@phosphor-icons/react/dist/ssr'
import { DockAiChatTab } from './DockAiChatTab'

export interface DockTab {
  id: string
  labelKey: 'dock.tabs.aiChat'
  icon: ComponentType<{ className?: string }>
  component: ComponentType
}

export const DOCK_TABS: readonly DockTab[] = [
  {
    id: 'ai-chat',
    labelKey: 'dock.tabs.aiChat',
    icon: ChatCircleTextIcon,
    component: DockAiChatTab,
  },
]
