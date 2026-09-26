import { RefreshCw } from 'lucide-react'
import { useEffect } from 'react'
import { Brand } from './components/Brand'
import { Shell } from './components/Shell'
import { useDashboard } from './hooks/useDashboard'
import { useRoute } from './hooks/useRoute'
import { systemHealth } from './lib/present'
import { ActivityPage } from './pages/ActivityPage'
import { GameDetailPage } from './pages/GameDetailPage'
import { LibraryPage } from './pages/LibraryPage'
import { OnboardingPage } from './pages/OnboardingPage'
import { ReviewPage } from './pages/ReviewPage'
import { SettingsPage } from './pages/SettingsPage'
import { SystemPage } from './pages/SystemPage'

// Earlier versions had separate pages for these; keep old links working.
const legacyRoutes: Record<string, string> = {
  '/conflicts': '/review',
  '/unassigned': '/review',
  '/devices': '/system',
  '/diagnostics': '/system',
}

export function App() {
  const { path, navigate } = useRoute()
  const { data, loading, error, isDemo, liveConnected, refresh } = useDashboard()

  useEffect(() => {
    if (legacyRoutes[path]) navigate(legacyRoutes[path], { replace: true })
  }, [navigate, path])

  useEffect(() => {
    if (data && !data.onboarding.complete && path !== '/onboarding') navigate('/onboarding')
  }, [data, navigate, path])

  useEffect(() => {
    const titles: Record<string, string> = { '/review': 'Review', '/activity': 'Activity', '/system': 'System', '/settings': 'Settings', '/onboarding': 'Setup' }
    const section = titles[path]
    document.title = section ? `${section} · ThorSync` : 'ThorSync'
  }, [path])

  if (loading || !data) {
    if (error) {
      return (
        <div className="standalone">
          <Brand />
          <h1>ThorSync couldn’t start</h1>
          <p>{error}</p>
          <button className="button button--primary" type="button" onClick={() => void refresh()}><RefreshCw size={16} />Try again</button>
        </div>
      )
    }
    return (
      <div className="standalone" aria-label="Loading ThorSync">
        <Brand />
        <div className="standalone__bar"><span /></div>
        <p>Opening the archive…</p>
      </div>
    )
  }

  if (path === '/onboarding') {
    return (
      <OnboardingPage
        endpoints={data.endpoints}
        diagnostics={data.diagnostics}
        archive={data.archive}
        demoMode={isDemo}
        canExit={data.onboarding.complete}
        navigate={navigate}
      />
    )
  }

  const page = (() => {
    if (path === '/review') return <ReviewPage conflicts={data.conflicts} files={data.unassigned} games={data.games} profiles={data.profiles} demoMode={isDemo} navigate={navigate} onRefresh={refresh} />
    if (path === '/activity') return <ActivityPage items={data.activity} navigate={navigate} />
    if (path === '/system') return <SystemPage endpoints={data.endpoints} checks={data.diagnostics} archive={data.archive} onRefresh={refresh} />
    if (path === '/settings') return <SettingsPage propagationEnabled={data.onboarding.propagationEnabled} quotaBytes={data.archive.quotaBytes} emulatorSettings={data.emulatorSettings} onSaved={refresh} navigate={navigate} demoMode={isDemo} />
    if (path.startsWith('/games/')) {
      const gameId = decodeURIComponent(path.slice('/games/'.length))
      return <GameDetailPage key={gameId} gameId={gameId} navigate={navigate} demoMode={isDemo} />
    }
    return <LibraryPage games={data.games} archive={data.archive} conflictCount={data.conflicts.length} unassignedCount={data.unassigned.length} demoMode={isDemo} navigate={navigate} onRefresh={refresh} />
  })()

  return (
    <Shell
      path={path}
      navigate={navigate}
      reviewCount={data.conflicts.length + data.unassigned.length}
      isDemo={isDemo}
      liveConnected={liveConnected}
      health={systemHealth(data.diagnostics)}
    >
      {page}
    </Shell>
  )
}
