import { useCallback, useEffect, useState } from 'react'

export interface RouteState {
  path: string
  navigate: (path: string) => void
}

export function useRoute(): RouteState {
  const [path, setPath] = useState(() => window.location.pathname || '/')

  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname || '/')
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  const navigate = useCallback((nextPath: string) => {
    if (window.location.pathname !== nextPath) window.history.pushState({}, '', nextPath)
    setPath(nextPath)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  return { path, navigate }
}
