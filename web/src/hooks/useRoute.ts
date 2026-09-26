import { useCallback, useEffect, useState } from 'react'

export interface RouteState {
  path: string
  navigate: (path: string, options?: { replace?: boolean }) => void
}

export function useRoute(): RouteState {
  const [path, setPath] = useState(() => window.location.pathname || '/')

  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname || '/')
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  const navigate = useCallback((nextPath: string, options?: { replace?: boolean }) => {
    if (options?.replace) window.history.replaceState({}, '', nextPath)
    else if (window.location.pathname !== nextPath) window.history.pushState({}, '', nextPath)
    setPath(nextPath)
    if (!options?.replace) window.scrollTo({ top: 0 })
  }, [])

  return { path, navigate }
}
