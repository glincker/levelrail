import {
  BracketsCurlyIcon,
  CalendarCheckIcon,
  ChatCircleDotsIcon,
  EnvelopeSimpleIcon,
  FolderOpenIcon,
  GlobeHemisphereWestIcon,
  LinkSimpleIcon,
  PackageIcon,
  CodeIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
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
  Icon: Icon
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
  Icon: Icon,
): Preset {
  return { id, kind, runtime, port: 8080, idleMinutes, suggestedName, Icon }
}

export const PRESETS: Preset[] = [
  preset('go', 'starter', 'go', NORMAL, 'go-handler', BracketsCurlyIcon),
  preset(
    'node',
    'starter',
    'node',
    NORMAL,
    'node-handler',
    GlobeHemisphereWestIcon,
  ),
  preset('python', 'starter', 'python', NORMAL, 'python-handler', CodeIcon),
  preset(
    'webhook',
    'idea',
    'node',
    QUICK,
    'webhook-receiver',
    WebhooksLogoIcon,
  ),
  preset('scheduled', 'idea', 'node', LONG, 'scheduled-job', CalendarCheckIcon),
  preset('form', 'idea', 'node', NORMAL, 'form-to-email', EnvelopeSimpleIcon),
  preset('shortener', 'idea', 'node', LONG, 'url-shortener', LinkSimpleIcon),
  preset('chatbot', 'idea', 'node', NORMAL, 'chat-bot', ChatCircleDotsIcon),
  preset('repo', 'repo', '', NORMAL, '', FolderOpenIcon),
  preset('image', 'image', '', NORMAL, '', PackageIcon),
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
