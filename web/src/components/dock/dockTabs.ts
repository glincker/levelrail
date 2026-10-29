import { lazy, type ComponentType, type LazyExoticComponent } from 'react'
import { MessageCircleGlassIcon, TerminalGlassIcon } from '@thesvg/ui-react'
import type { ExperimentalFeature } from '../../lib/experimental'

// Lazy so xterm and the chat stack load when a tab first opens, not on every page.
const DockAiChatTab = lazy(() =>
  import('./DockAiChatTab').then((m) => ({ default: m.DockAiChatTab })),
)
const DockConsoleTab = lazy(() =>
  import('./DockConsoleTab').then((m) => ({ default: m.DockConsoleTab })),
)

export interface DockTab {
  id: string
  labelKey: 'dock.tabs.aiChat' | 'dock.tabs.console'
  icon: ComponentType<{ className?: string }>
  component: ComponentType | LazyExoticComponent<ComponentType>
  // Hidden from the tab strip until the feature is switched on.
  feature?: ExperimentalFeature
}

export const DOCK_TABS: readonly DockTab[] = [
  {
    id: 'ai-chat',
    labelKey: 'dock.tabs.aiChat',
    icon: MessageCircleGlassIcon,
    component: DockAiChatTab,
  },
  {
    id: 'console',
    labelKey: 'dock.tabs.console',
    icon: TerminalGlassIcon,
    component: DockConsoleTab,
    feature: 'cli-console',
  },
]
