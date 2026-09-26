import type { DeliveryState, DiagnosticCheck, Game, HealthState } from '../types'

// Book-cloth colours for generated covers. They are chosen per game rather than
// taken from the API accent, which is only a per-platform hint.
const coverCloths = ['#7a2e2a', '#2f5845', '#27405e', '#8c6322', '#4a5560', '#5a3a55', '#2d5f63', '#9a4b2f']

function hash(value: string): number {
  let result = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    result ^= value.charCodeAt(index)
    result = Math.imul(result, 16777619)
  }
  return result >>> 0
}

export function coverCloth(seed: string): string {
  return coverCloths[hash(seed) % coverCloths.length]
}

export function initialOf(title: string): string {
  const match = title.match(/[\p{L}\p{N}]/u)
  return match ? match[0].toUpperCase() : '?'
}

export const deliveryLabels: Record<DeliveryState, string> = {
  delivered: 'In step',
  syncing: 'Delivering',
  paused: 'Paused',
  missing: 'Missing',
}

export function needsAttention(game: Game): boolean {
  return game.hasConflict || game.deliveryState === 'missing'
}

export function gameStateLine(game: Game): string {
  if (game.hasConflict) return game.healthMessage ?? 'Two versions need your decision'
  if (game.healthMessage) return game.healthMessage
  if (game.deliveryState === 'delivered') return 'Same save on both devices'
  if (game.deliveryState === 'syncing') return 'Delivery in progress'
  if (game.deliveryState === 'missing') return 'A device copy is missing'
  return 'Delivery paused'
}

export interface SystemHealth {
  state: HealthState
  label: string
  failing: DiagnosticCheck[]
}

export function systemHealth(checks: DiagnosticCheck[]): SystemHealth {
  const offline = checks.filter((check) => check.state === 'offline')
  const degraded = checks.filter((check) => check.state === 'degraded')
  if (offline.length) return { state: 'offline', label: `${offline.length} check${offline.length === 1 ? '' : 's'} failing`, failing: [...offline, ...degraded] }
  if (degraded.length) return { state: 'degraded', label: `${degraded.length} check${degraded.length === 1 ? '' : 's'} degraded`, failing: degraded }
  return { state: 'healthy', label: 'All systems normal', failing: [] }
}

export function dayLabel(value: string): string {
  const date = new Date(value)
  const today = new Date()
  const startOf = (day: Date) => new Date(day.getFullYear(), day.getMonth(), day.getDate()).getTime()
  const difference = Math.round((startOf(today) - startOf(date)) / 86_400_000)
  if (difference === 0) return 'Today'
  if (difference === 1) return 'Yesterday'
  return new Intl.DateTimeFormat(undefined, { weekday: 'long', month: 'long', day: 'numeric' }).format(date)
}

export function clockTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

export function shortDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(value))
}
