import { describe, expect, it } from 'vitest'
import { isShellCommand } from './shellCommand'

describe('isShellCommand', () => {
  it.each([
    ['sudo ufw allow 443', true],
    ['levelrail-cli firewall exposure', true],
    ['LEVELRAIL_SOCKET_ACTIVATION=1 ./install.sh upgrade', true],
    ['Confirm your router forwards port 80', false],
    ['Install and enable an NTP client', false],
  ])('%s', (text, want) => {
    expect(isShellCommand(text)).toBe(want)
  })
})
