import type { ComponentType } from 'react'
import {
  ChatCircleTextIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { ExperimentalFeature } from '../../lib/experimental'
import { DockAiChatTab } from './DockAiChatTab'
import { DockConsoleTab } from './DockConsoleTab'

export interface DockTab {
  id: string
  labelKey: 'dock.tabs.aiChat' | 'dock.tabs.console'
  icon: ComponentType<{ className?: string }>
  component: ComponentType
  // Hidden from the tab strip until the feature is switched on.
  feature?: ExperimentalFeature
}

export const DOCK_TABS: readonly DockTab[] = [
  {
    id: 'ai-chat',
    labelKey: 'dock.tabs.aiChat',
    icon: ChatCircleTextIcon,
    component: DockAiChatTab,
  },
  {
    id: 'console',
    labelKey: 'dock.tabs.console',
    icon: TerminalWindowIcon,
    component: DockConsoleTab,
    feature: 'cli-console',
  },
]
