import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import en from '../../locales/en/upgradeSafety.json'
import type { SelfUpgradePlan } from '../../queries/selfUpgrade'
import { UpgradeSafetyCard } from './UpgradeSafetyCard'
import { upgradeBlockReason } from './upgradeBlockReason'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['upgradeSafety'],
  defaultNS: 'upgradeSafety',
  resources: { en: { upgradeSafety: en } },
  interpolation: { escapeValue: false },
})

const GIB = 1024 ** 3
const plan: SelfUpgradePlan = {
  current_version: 'v1.0.0',
  target_version: 'v1.1.0',
  breaking: [],
  notes_available: true,
  can_apply: true,
  command: 'levelrail upgrade',
  steps: [],
  pending_migrations: 3,
  disk: {
    known: true,
    free_bytes: 10 * GIB,
    required_bytes: 2 * GIB,
    db_bytes: GIB,
    ok: true,
  },
  downtime_seconds: 20,
  restart_behaviour: 'systemd restart',
  backup: {
    will_take: true,
    dir: '/var/lib/lr/backups',
    restore_command: 'levelrail restore --latest',
  },
}

function view(p: SelfUpgradePlan, onSkip = vi.fn(), failed = false) {
  return render(
    <I18nextProvider i18n={i18n}>
      <UpgradeSafetyCard
        plan={p}
        skipBackup={false}
        onSkipBackupChange={onSkip}
        backupFailed={failed}
      />
    </I18nextProvider>,
  )
}

describe('UpgradeSafetyCard', () => {
  it('shows changes, checks, downtime and the restore command', () => {
    view(plan)
    expect(screen.getByText('3 database migrations')).toBeInTheDocument()
    expect(screen.getByText(/Disk OK: 10 GiB free/)).toBeInTheDocument()
    expect(screen.getByText(/About 20 s offline/)).toBeInTheDocument()
    expect(screen.getByText(/levelrail restore --latest/)).toBeInTheDocument()
  })

  it('keeps skip backup behind an expander, off by default', async () => {
    const onSkip = vi.fn()
    view(plan, onSkip)
    expect(screen.queryByRole('checkbox')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    const box = screen.getByRole('checkbox', {
      name: /Skip the pre-upgrade backup/,
    })
    expect(box).not.toBeChecked()
    await userEvent.click(box)
    expect(onSkip).toHaveBeenCalledWith(true)
  })

  it('opens the skip option and explains next steps after a backup failure', () => {
    view(plan, vi.fn(), true)
    expect(screen.getByRole('alert')).toHaveTextContent(/backup failed/)
    expect(
      screen.getByRole('checkbox', { name: /Skip the pre-upgrade backup/ }),
    ).toBeVisible()
  })

  it('blocks the upgrade with the numbers when the disk is short', () => {
    const short = {
      ...plan,
      disk: {
        ...plan.disk!,
        ok: false,
        free_bytes: GIB,
        required_bytes: 3 * GIB,
      },
    }
    expect(upgradeBlockReason(short)).toEqual({
      kind: 'disk',
      shortBytes: 2 * GIB,
    })
    expect(upgradeBlockReason(plan)).toBeNull()
  })
})
