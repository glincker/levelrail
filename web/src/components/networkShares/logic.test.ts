import { describe, expect, it } from 'vitest'
import {
  isValidHost,
  normalizePath,
  remoteAddress,
  suggestName,
  toggleOption,
  validate,
  type ShareValues,
} from './logic'

const base: ShareValues = {
  protocol: 'nfs',
  name: 'media-nas',
  host: 'nas.lan',
  path: '/export/media',
  username: '',
  password: '',
  options: '',
}

describe('isValidHost', () => {
  it.each([
    'nas.lan',
    'nas',
    '192.168.1.50',
    '::1',
    '[fe80::1]',
    '2001:db8::8a2e:370:7334',
  ])('accepts %s', (h) => {
    expect(isValidHost(h)).toBe(true)
  })
  it.each([
    '',
    'bad host',
    '300.1.1.1',
    '1.2.3',
    'nas_.lan',
    '-nas',
    'a..b',
    ':::',
    '1:2:3',
  ])('rejects %s', (h) => {
    expect(isValidHost(h)).toBe(false)
  })
})

describe('path validation', () => {
  it('requires a leading slash for nfs', () => {
    expect(validate({ ...base, path: 'export' }, []).path).toBe('nfsPathFormat')
    expect(validate(base, []).path).toBeUndefined()
  })
  it('wants a bare share name for smb and normalizes the slash', () => {
    const smb = {
      ...base,
      protocol: 'cifs' as const,
      username: 'u',
      password: 'p',
    }
    expect(validate({ ...smb, path: 'a/b' }, []).path).toBe('smbPathFormat')
    expect(validate({ ...smb, path: 'backups' }, []).path).toBeUndefined()
    expect(normalizePath('cifs', 'backups')).toBe('/backups')
    expect(normalizePath('cifs', '/backups')).toBe('/backups')
  })
  it('requires credentials only for smb', () => {
    expect(validate(base, [])).toEqual({})
    const errs = validate({ ...base, protocol: 'cifs' }, [])
    expect(errs.username).toBe('usernameRequired')
    expect(errs.password).toBe('passwordRequired')
  })
})

describe('name', () => {
  it('suggests kebab-case from path and host', () => {
    expect(suggestName('nas.lan', '/export/media')).toBe('media-nas')
    expect(suggestName('NAS.lan', '/Export/My Media/')).toBe('my-media-nas')
    expect(suggestName('10.0.0.5', '/data')).toBe('data-10-0-0-5')
    expect(suggestName('', '')).toBe('')
  })
  it('flags a taken name regardless of case', () => {
    expect(validate(base, ['Media-NAS']).name).toBe('nameTaken')
    expect(validate({ ...base, name: 'Bad Name' }, []).name).toBe('nameFormat')
  })
})

describe('options and summary', () => {
  it('toggles chips and keeps nfs versions exclusive', () => {
    let o = toggleOption('', 'ro')
    o = toggleOption(o, 'nfsvers=4')
    expect(o).toBe('ro,nfsvers=4')
    o = toggleOption(o, 'nfsvers=3')
    expect(o).toBe('ro,nfsvers=3')
    expect(toggleOption(o, 'ro')).toBe('nfsvers=3')
    expect(toggleOption('', 'uid=1000,gid=1000')).toBe('uid=1000,gid=1000')
  })
  it('formats the remote address per protocol', () => {
    expect(remoteAddress('nfs', 'nas.lan', '/export/media')).toBe(
      'nas.lan:/export/media',
    )
    expect(remoteAddress('cifs', 'nas.lan', 'backups')).toBe(
      '//nas.lan/backups',
    )
  })
})
