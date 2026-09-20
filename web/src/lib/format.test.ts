import { describe, expect, it } from 'vitest'
import { fileSize, timeAgo } from './format'

describe('format helpers', () => {
  it('formats archive sizes for people', () => {
    expect(fileSize(1024)).toBe('1.0 KB')
    expect(fileSize(5 * 1024 * 1024)).toBe('5.0 MB')
  })

  it('does not return an invalid relative time', () => {
    expect(timeAgo(new Date().toISOString())).toBe('Just now')
  })
})
