import { test } from 'node:test'
import assert from 'node:assert/strict'
import { presetRange, customRange, toDateInput, DAY } from '../src/lib/trafficDates.ts'

const now = new Date('2026-11-14T09:35:20Z')
test('presets use Taipei calendar boundaries independently of host timezone', () => {
  assert.equal(presetRange('today', now).start, '2026-11-13T16:00:00.000Z')
  assert.equal(presetRange('yesterday', now).end, '2026-11-13T16:00:00.000Z')
  assert.equal(presetRange('7d', now).start, '2026-11-07T16:00:00.000Z')
  assert.equal(presetRange('90d', now).start, '2026-08-16T16:00:00.000Z')
  const rolling = presetRange('24h', now)
  assert.equal(new Date(rolling.end) - new Date(rolling.start), DAY)
})
test('leap day, month and year transitions', () => {
  assert.equal(presetRange('yesterday', new Date('2028-03-01T02:00:00Z')).start, '2028-02-28T16:00:00.000Z')
  assert.equal(presetRange('3d', new Date('2027-01-01T02:00:00Z')).start, '2026-12-29T16:00:00.000Z')
})
test('inclusive end minute becomes exclusive API boundary; current minute clamps to now', () => {
  const range = customRange('2026-11-13T00:00', '2026-11-13T23:59', now)
  assert.equal(range.end, '2026-11-13T16:00:00.000Z')
  assert.equal(new Date(range.end) - new Date(range.start), DAY)
  assert.equal(customRange('2026-11-14T00:00', toDateInput(now), now).end, now.toISOString())
})
test('reject invalid, reversed, future and oversized ranges', () => {
  for (const [start, end] of [['', ''], ['2026-02-30T00:00', '2026-03-01T00:00'], ['2026-11-14T12:00', '2026-11-13T12:00'], ['2026-11-14T00:00', '2026-11-15T00:00'], ['2026-01-01T00:00', '2026-11-14T00:00']]) {
    assert.throws(() => customRange(start, end, now))
  }
})
