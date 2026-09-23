import { describe, expect, it } from 'vitest'
import { gitHostIconName } from './gitHost'

describe('gitHostIconName', () => {
  it.each([
    ['https://github.com/org/repo.git', 'github'],
    ['https://www.github.com/org/repo', 'github'],
    ['github.com/org/repo', 'github'],
    ['git@github.com:org/repo.git', 'github'],
    ['ssh://git@gitlab.com/org/repo.git', 'gitlab'],
    ['https://bitbucket.org/org/repo', 'bitbucket'],
    ['https://github.com.evil.test/org/repo', null],
    ['https://evil.test/github.com/org/repo', null],
    ['https://evil.test/?u=gitlab.com', null],
    ['git@evil.test:github.com/repo.git', null],
    ['https://git.example.com/org/repo', null],
    ['', null],
  ])('%s -> %s', (url, want) => {
    expect(gitHostIconName(url)).toBe(want)
  })
})
