import { queryOptions, useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type PanelSlot = 'plugins' | 'app-detail' | 'settings' | 'dashboard'

export type WidgetType =
  'stat' | 'table' | 'list' | 'chart' | 'markdown' | 'status' | 'button'

export interface PanelWidgetDef {
  type: WidgetType
  title: string
}

export interface PluginPanelEntry {
  plugin: string
  publisher: string
  panel: {
    id: string
    title: string
    description: string
    slot: PanelSlot
    widgets: PanelWidgetDef[]
  }
}

export type PanelStatusState = 'ok' | 'warning' | 'error' | 'unknown'

export interface WidgetView {
  type: WidgetType
  title: string
  error?: string
  value?: string
  unit?: string
  state?: PanelStatusState
  text?: string
  columns?: { key: string; label: string }[]
  rows?: string[][]
  items?: string[]
  points?: { label: string; value: number }[]
  action?: string
  label?: string
  confirm?: string
}

export interface PanelView {
  widgets: WidgetView[]
}

export interface PanelActionResult {
  ok: boolean
  message: string
}

export const pluginPanelKeys = {
  slot: (slot: PanelSlot) => ['plugin-panels', slot] as const,
  data: (plugin: string, panel: string, appId: string | undefined) =>
    ['plugin-panel-data', plugin, panel, appId ?? ''] as const,
}

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    cache: 'no-store',
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function pluginPanelsQueryOptions(slot: PanelSlot) {
  return queryOptions({
    queryKey: pluginPanelKeys.slot(slot),
    queryFn: () =>
      send<PluginPanelEntry[]>(
        'GET',
        `/api/v1/plugin-panels?slot=${encodeURIComponent(slot)}`,
      ),
    staleTime: 30_000,
    retry: false,
  })
}

export function usePluginPanels(slot: PanelSlot) {
  return useQuery(pluginPanelsQueryOptions(slot))
}

function panelPath(plugin: string, panel: string): string {
  return `/api/v1/plugins/${encodeURIComponent(plugin)}/panels/${encodeURIComponent(panel)}`
}

export function usePanelData(
  plugin: string,
  panel: string,
  appId: string | undefined,
) {
  const query = appId ? `?app_id=${encodeURIComponent(appId)}` : ''
  return useQuery({
    queryKey: pluginPanelKeys.data(plugin, panel, appId),
    queryFn: () =>
      send<PanelView>('GET', `${panelPath(plugin, panel)}/data${query}`),
    staleTime: 15_000,
    refetchInterval: 30_000,
    retry: false,
  })
}

export function usePanelAction(plugin: string, panel: string) {
  return useMutation({
    mutationFn: (input: { action: string; appId?: string }) =>
      send<PanelActionResult>(
        'POST',
        `${panelPath(plugin, panel)}/actions/${encodeURIComponent(input.action)}`,
        input.appId ? { app_id: input.appId } : {},
      ),
  })
}
