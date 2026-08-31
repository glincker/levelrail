import { describe, expect, it } from 'vitest'
import { fromCron, scheduleRetentionSummary, toCron } from './cronSchedule'
import type { ScheduleFormValues } from './cronSchedule'

describe('toCron', () => {
  it('builds a daily cron string from time', () => {
    const values: ScheduleFormValues = {
      targetId: 't1',
      frequency: 'daily',
      time: '03:30',
      weekday: '0',
      customCron: '',
      retain: '7',
      retainDays: '0',
    }
    expect(toCron(values)).toBe('30 3 * * *')
  })

  it('builds a weekly cron string from time and weekday', () => {
    const values: ScheduleFormValues = {
      targetId: 't1',
      frequency: 'weekly',
      time: '00:05',
      weekday: '3',
      customCron: '',
      retain: '7',
      retainDays: '0',
    }
    expect(toCron(values)).toBe('5 0 * * 3')
  })

  it('passes a custom cron string through as-is, trimmed', () => {
    const values: ScheduleFormValues = {
      targetId: 't1',
      frequency: 'custom',
      time: '',
      weekday: '0',
      customCron: '  */15 * * * *  ',
      retain: '7',
      retainDays: '0',
    }
    expect(toCron(values)).toBe('*/15 * * * *')
  })
})

describe('fromCron', () => {
  it('recognizes a daily schedule', () => {
    expect(fromCron('30 3 * * *')).toEqual({
      frequency: 'daily',
      time: '03:30',
      weekday: '0',
      customCron: '30 3 * * *',
    })
  })

  it('recognizes a weekly schedule', () => {
    expect(fromCron('5 0 * * 3')).toEqual({
      frequency: 'weekly',
      time: '00:05',
      weekday: '3',
      customCron: '5 0 * * 3',
    })
  })

  it('falls back to custom for anything else', () => {
    expect(fromCron('*/15 * * * *')).toEqual({
      frequency: 'custom',
      time: '03:00',
      weekday: '0',
      customCron: '*/15 * * * *',
    })
  })

  it('falls back to custom defaults for an empty/undefined cron', () => {
    expect(fromCron(undefined)).toEqual({
      frequency: 'custom',
      time: '03:00',
      weekday: '0',
      customCron: '',
    })
  })

  it('round-trips through toCron for a daily schedule', () => {
    const parsed = fromCron('30 3 * * *')
    const roundTripped = toCron({
      targetId: 't1',
      retain: '0',
      retainDays: '0',
      ...parsed,
    })
    expect(roundTripped).toBe('30 3 * * *')
  })
})

describe('scheduleRetentionSummary', () => {
  it('describes keeping every backup when both are unset', () => {
    expect(scheduleRetentionSummary(undefined, undefined)).toBe(
      'keeping every backup',
    )
  })

  it('describes a count limit', () => {
    expect(scheduleRetentionSummary(7, 0)).toBe('keeping the last 7 backups')
  })

  it('describes an age limit', () => {
    expect(scheduleRetentionSummary(0, 30)).toBe(
      'deleting backups older than 30 days',
    )
  })

  it('describes both limits together', () => {
    expect(scheduleRetentionSummary(7, 30)).toBe(
      'keeping the last 7 backups, deleting backups older than 30 days',
    )
  })
})
