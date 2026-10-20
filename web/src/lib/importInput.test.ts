import { describe, expect, it } from 'vitest'
import { guessImportSource } from './importInput'

describe('guessImportSource', () => {
  it.each([
    ['', null],
    ['https://github.com/o/r', 'repo'],
    ['github.com/o/r', 'repo'],
    ['git@github.com:o/r.git', 'repo'],
    ['nginx', 'image'],
    ['ghcr.io/o/app:1.2', 'image'],
    ['docker run -p 80:80 nginx', 'docker_run'],
    ['sudo docker run nginx', 'docker_run'],
    ['docker container run nginx', 'docker_run'],
    ['services:\n  web:\n    image: x\n', 'compose'],
    ['FROM alpine\nRUN echo hi\n', 'dockerfile'],
    ['hello there', null],
  ])('%j -> %s', (input, want) => {
    expect(guessImportSource(input)).toBe(want)
  })
})
