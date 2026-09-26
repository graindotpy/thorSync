import type { DeliveryState, HealthState } from '../types'
import { deliveryLabels } from '../lib/present'

export function DeliveryBadge({ state }: { state: DeliveryState }) {
  return (
    <span className={`state state--${state}`}>
      <span className="state__dot" aria-hidden="true" />
      {deliveryLabels[state]}
    </span>
  )
}

const healthLabels: Record<HealthState, string> = {
  healthy: 'Healthy',
  degraded: 'Degraded',
  offline: 'Offline',
}

export function HealthBadge({ state }: { state: HealthState }) {
  return (
    <span className={`state state--${state}`}>
      <span className="state__dot" aria-hidden="true" />
      {healthLabels[state]}
    </span>
  )
}
