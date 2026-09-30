import {
  PhRocketLaunch,
  PhBookOpen,
  PhCode,
  PhLightbulb,
  PhCompass,
  PhActivity,
  PhStack,
} from '@phosphor-icons/vue'
import type { Component } from 'vue'

export const sectionIcons: Record<string, Component> = {
  Tutorials: PhRocketLaunch,
  'How-to guides': PhBookOpen,
  Reference: PhCode,
  Explanation: PhLightbulb,
  'Design proposals': PhCompass,
  Status: PhActivity,
  'Docs index': PhStack,
}

export const defaultSectionIcon = PhStack
