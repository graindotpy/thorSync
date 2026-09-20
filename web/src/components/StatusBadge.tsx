import { AlertTriangle, Check, Clock3, MinusCircle, RefreshCw } from 'lucide-react'
import type { DeliveryState, HealthState } from '../types'

const deliveryLabels: Record<DeliveryState, string> = {
  delivered: 'Everywhere',
  syncing: 'Delivering',
  paused: 'Paused',
  missing: 'Missing',
}

export function DeliveryBadge({ state }: { state: DeliveryState }) {
  const Icon =
    state === 'delivered'
      ? Check
      : state === 'syncing'
        ? RefreshCw
        : state === 'paused'
          ? AlertTriangle
          : MinusCircle
  return (
    <span className={`status-badge status-badge--${state}`}>
      <Icon size={12} className={state === 'syncing' ? 'spin-slow' : ''} />
      {deliveryLabels[state]}
    </span>
  )
}

export function HealthBadge({ state }: { state: HealthState }) {
  const Icon = state === 'healthy' ? Check : state === 'degraded' ? Clock3 : MinusCircle
  return (
    <span className={`health-badge health-badge--${state}`}>
      <Icon size={13} />
      {state}
    </span>
  )
}
