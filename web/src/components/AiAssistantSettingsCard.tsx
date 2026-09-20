import { useState, type FormEvent } from 'react'
import {
  CheckCircleIcon,
  MinusCircleIcon,
  RobotIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { AiAssistantSettings } from '../queries/aiAssistantSettings'
import {
  useClearAiAssistantSettings,
  useUpdateAiAssistantSettings,
} from '../queries/aiAssistantSettings'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { SettingsFormActions, SettingsFormAlerts } from './SettingsCard'

// BYOK provider config for the AI assistant chat feature: GET/PUT/DELETE
// /api/v1/settings/ai-assistant. api_key is write-only, following the
// same pattern EmailSettingsCard/CloudflareTunnelCard already use for
// smtp_password/tunnel token: never echoed back, `configured` reports
// presence instead. Only "anthropic" is a supported provider today, so
// the select has one entry rather than being disabled outright, keeping
// the request shape stable if a second provider lands later.
export function AiAssistantSettingsCard({
  settings,
}: {
  settings: AiAssistantSettings
}) {
  const updateSettings = useUpdateAiAssistantSettings()
  const clearSettings = useClearAiAssistantSettings()

  const [provider, setProvider] = useState(settings.provider || 'anthropic')
  const [model, setModel] = useState(settings.model)
  const [apiKey, setApiKey] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  const pending = updateSettings.isPending || clearSettings.isPending

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (!model.trim()) {
      setFormError('A model is required.')
      return
    }
    if (!settings.configured && !apiKey.trim()) {
      setFormError('An API key is required to configure the AI assistant.')
      return
    }
    updateSettings.mutate(
      { provider, model: model.trim(), api_key: apiKey.trim() },
      {
        onSuccess: () => {
          setApiKey('')
          toast.add({ title: 'AI assistant settings saved.', type: 'success' })
        },
      },
    )
  }

  function handleClear() {
    clearSettings.mutate(undefined, {
      onSuccess: () => {
        setApiKey('')
        toast.add({ title: 'AI assistant API key cleared.', type: 'success' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RobotIcon className="size-4" />
          AI Assistant
          <StatusBadge
            variant={settings.configured ? 'success' : 'muted'}
            label={settings.configured ? 'Configured' : 'Not configured'}
            icon={settings.configured ? CheckCircleIcon : MinusCircleIcon}
          />
        </CardTitle>
        <CardDescription>
          Bring your own LLM API key to chat with an assistant that reads logs,
          metrics, and deploy history through the platform&apos;s own MCP tools.
          It always pauses for your approval before any action that changes
          state (deploy, rollback, restart).
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-5">
          <Field>
            <FieldLabel htmlFor="ai-assistant-provider">Provider</FieldLabel>
            <Select
              value={provider}
              onValueChange={(value) => {
                setProvider(value ?? 'anthropic')
              }}
            >
              <SelectTrigger
                id="ai-assistant-provider"
                className="w-full sm:w-64"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="anthropic">Anthropic</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field>
            <FieldLabel htmlFor="ai-assistant-model">Model</FieldLabel>
            <Input
              id="ai-assistant-model"
              value={model}
              onChange={(e) => {
                setModel(e.target.value)
              }}
              placeholder="claude-sonnet-4-5"
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="ai-assistant-api-key">API key</FieldLabel>
            <Input
              id="ai-assistant-api-key"
              type="password"
              autoComplete="off"
              value={apiKey}
              onChange={(e) => {
                setApiKey(e.target.value)
              }}
              placeholder={
                settings.configured ? '••••••••••••' : 'Paste your API key'
              }
            />
            <FieldDescription>
              {settings.configured
                ? 'A key is already configured. Leave blank to keep it.'
                : 'No key set yet. Never sent back once saved.'}
            </FieldDescription>
          </Field>

          <SettingsFormAlerts
            alerts={[
              formError ? { key: 'form', message: formError } : null,
              updateSettings.isError
                ? { key: 'update', message: updateSettings.error.message }
                : null,
              clearSettings.isError
                ? { key: 'clear', message: clearSettings.error.message }
                : null,
            ]}
          />

          <SettingsFormActions
            pending={pending}
            savePending={updateSettings.isPending}
            showSecondary={settings.configured}
            secondaryPending={clearSettings.isPending}
            secondaryLabel="Clear key"
            secondaryPendingLabel="Clearing..."
            onSecondaryClick={handleClear}
          />
        </form>
      </CardContent>
    </Card>
  )
}
