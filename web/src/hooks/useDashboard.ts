import { useCallback, useEffect, useState } from 'react'
import { loadDashboard, subscribeToEvents } from '../lib/api'
import type { DashboardData } from '../types'

interface DashboardState {
  data: DashboardData | null
  loading: boolean
  isDemo: boolean
  liveConnected: boolean
  error: string | null
  refresh: () => Promise<void>
}

export function useDashboard(): DashboardState {
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [isDemo, setIsDemo] = useState(false)
  const [liveConnected, setLiveConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const result = await loadDashboard()
      setData(result.data)
      setIsDemo(result.isDemo)
      setError(null)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load ThorSync')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    const unsubscribe = subscribeToEvents(
      () => void refresh(),
      (connected) => setLiveConnected(connected),
    )
    return unsubscribe
  }, [refresh])

  return { data, loading, isDemo, liveConnected, error, refresh }
}
