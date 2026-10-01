import * as React from 'react'
import { readJson, writeJson } from './persist'

export const TOAST_HISTORY_STORAGE_KEY = 'shell.notifications.toastHistory'

const MAX_TOAST_HISTORY = 50

export interface ToastHistoryEntry {
  id: string
  title: string
  description?: string
  timestamp: number
}

const isToastHistoryEntry = (v: unknown): v is ToastHistoryEntry => {
  if (typeof v !== 'object' || v === null) return false
  const e = v as Record<string, unknown>
  return (
    typeof e.id === 'string' &&
    typeof e.title === 'string' &&
    typeof e.timestamp === 'number' &&
    (e.description === undefined || typeof e.description === 'string')
  )
}

const isToastHistoryList = (v: unknown): v is ToastHistoryEntry[] =>
  Array.isArray(v) && v.every(isToastHistoryEntry)

let current: readonly ToastHistoryEntry[] | undefined
const listeners = new Set<() => void>()

function load(): readonly ToastHistoryEntry[] {
  current ??= readJson(TOAST_HISTORY_STORAGE_KEY, [], isToastHistoryList)
  return current
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

function notify(): void {
  listeners.forEach((l) => {
    l()
  })
}

function nextId(): string {
  const random =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID()
      : Math.random().toString(36).slice(2)
  return `${Date.now().toString(36)}-${random}`
}

export function recordToastHistory(entry: {
  title: string
  description?: string
}): void {
  const next: ToastHistoryEntry = {
    id: nextId(),
    title: entry.title,
    description: entry.description,
    timestamp: Date.now(),
  }
  current = [next, ...load()].slice(0, MAX_TOAST_HISTORY)
  writeJson(TOAST_HISTORY_STORAGE_KEY, current)
  notify()
}

export function clearToastHistory(): void {
  current = []
  writeJson(TOAST_HISTORY_STORAGE_KEY, current)
  notify()
}

export function resetToastHistoryForTests(): void {
  current = undefined
}

export function useToastHistory(): readonly ToastHistoryEntry[] {
  return React.useSyncExternalStore(subscribe, load, load)
}
