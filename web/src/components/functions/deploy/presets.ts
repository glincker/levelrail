import {
  BracesGlassIcon,
  CalendarCheckGlassIcon,
  CodeGlassIcon,
  FolderGlassIcon,
  GlobeGlassIcon,
  LinkGlassIcon,
  MailGlassIcon,
  MessageCircleGlassIcon,
  PackageGlassIcon,
  WebhookGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import type { IdleChoice } from './logic'

export type PresetId =
  | 'go'
  | 'node'
  | 'python'
  | 'webhook'
  | 'scheduled'
  | 'form'
  | 'shortener'
  | 'chatbot'
  | 'repo'
  | 'image'

export type PresetKind = 'starter' | 'idea' | 'repo' | 'image'

export interface Preset {
  id: PresetId
  kind: PresetKind
  /** Runtime id from GET /functions/runtimes; empty for repo and image. */
  runtime: string
  port: number
  idleMinutes: number
  suggestedName: string
  Icon: UiIcon
}

const QUICK = 5
const NORMAL = 10
const LONG = 60

function preset(
  id: PresetId,
  kind: PresetKind,
  runtime: string,
  idleMinutes: number,
  suggestedName: string,
  Icon: UiIcon,
): Preset {
  return { id, kind, runtime, port: 8080, idleMinutes, suggestedName, Icon }
}

export const PRESETS: Preset[] = [
  preset('go', 'starter', 'go', NORMAL, 'go-handler', BracesGlassIcon),
  preset('node', 'starter', 'node', NORMAL, 'node-handler', GlobeGlassIcon),
  preset(
    'python',
    'starter',
    'python',
    NORMAL,
    'python-handler',
    CodeGlassIcon,
  ),
  preset(
    'webhook',
    'idea',
    'node',
    QUICK,
    'webhook-receiver',
    WebhookGlassIcon,
  ),
  preset(
    'scheduled',
    'idea',
    'node',
    LONG,
    'scheduled-job',
    CalendarCheckGlassIcon,
  ),
  preset('form', 'idea', 'node', NORMAL, 'form-to-email', MailGlassIcon),
  preset('shortener', 'idea', 'node', LONG, 'url-shortener', LinkGlassIcon),
  preset('chatbot', 'idea', 'node', NORMAL, 'chat-bot', MessageCircleGlassIcon),
  preset('repo', 'repo', '', NORMAL, '', FolderGlassIcon),
  preset('image', 'image', '', NORMAL, '', PackageGlassIcon),
]

export function findPreset(id: string): Preset {
  return PRESETS.find((p) => p.id === id) ?? PRESETS[0]!
}

const KNOWN_IDLE: IdleChoice[] = ['5', '10', '60', '1440']

/** What a preset sets before the user touches anything. */
export function presetDefaults(preset: Preset, runtimePort?: number) {
  const asChoice = String(preset.idleMinutes) as IdleChoice
  return {
    source: preset.kind === 'image' ? ('image' as const) : ('repo' as const),
    name: preset.suggestedName,
    port: String(runtimePort ?? preset.port),
    idleChoice: KNOWN_IDLE.includes(asChoice) ? asChoice : ('custom' as const),
    customIdle: String(preset.idleMinutes),
  }
}
