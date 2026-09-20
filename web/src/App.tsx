import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useEffect } from 'react'
import { Brand } from './components/Brand'
import { Shell } from './components/Shell'
import { useDashboard } from './hooks/useDashboard'
import { useRoute } from './hooks/useRoute'
import { ActivityPage } from './pages/ActivityPage'
import { ConflictsPage } from './pages/ConflictsPage'
import { DevicesPage } from './pages/DevicesPage'
import { DiagnosticsPage } from './pages/DiagnosticsPage'
import { GameDetailPage } from './pages/GameDetailPage'
import { LibraryPage } from './pages/LibraryPage'
import { OnboardingPage } from './pages/OnboardingPage'
import { SettingsPage } from './pages/SettingsPage'
import { UnassignedPage } from './pages/UnassignedPage'

export function App() {
  const { path, navigate } = useRoute()
  const { data, loading, error, isDemo, liveConnected, refresh } = useDashboard()

  useEffect(() => {
    if (data && !data.onboarding.complete && path !== '/onboarding') navigate('/onboarding')
  }, [data, navigate, path])

  if (loading || !data) {
    if (error) {
      return (
        <div className="fatal-state">
          <span><AlertTriangle size={24} /></span>
          <Brand />
          <h1>ThorSync couldn’t start</h1>
          <p>{error}</p>
          <button className="button button--primary" type="button" onClick={() => void refresh()}><RefreshCw size={16} />Try again</button>
        </div>
      )
    }
    return (
      <div className="app-loading" aria-label="Loading ThorSync">
        <Brand />
        <div className="app-loading__bar"><span /></div>
        <p>Opening your save library…</p>
      </div>
    )
  }

  if (path === '/onboarding') {
    return <OnboardingPage endpoints={data.endpoints} demoMode={isDemo} navigate={navigate} />
  }

  const page = (() => {
    if (path === '/') return <LibraryPage games={data.games} archive={data.archive} navigate={navigate} />
    if (path === '/activity') return <ActivityPage items={data.activity} navigate={navigate} />
    if (path === '/conflicts') return <ConflictsPage conflicts={data.conflicts} demoMode={isDemo} navigate={navigate} />
    if (path === '/devices') return <DevicesPage endpoints={data.endpoints} navigate={navigate} onRefresh={refresh} />
    if (path === '/unassigned') return <UnassignedPage files={data.unassigned} games={data.games} onMapped={refresh} navigate={navigate} />
    if (path === '/diagnostics') return <DiagnosticsPage checks={data.diagnostics} archive={data.archive} onRefresh={refresh} />
    if (path === '/settings') return <SettingsPage propagationEnabled={data.onboarding.propagationEnabled} quotaBytes={data.archive.quotaBytes} onSaved={refresh} />
    if (path.startsWith('/games/')) {
      const gameId = decodeURIComponent(path.slice('/games/'.length))
      return <GameDetailPage gameId={gameId} navigate={navigate} demoMode={isDemo} />
    }
    return <LibraryPage games={data.games} archive={data.archive} navigate={navigate} />
  })()

  return (
    <Shell
      path={path}
      navigate={navigate}
      conflictCount={data.conflicts.length}
      isDemo={isDemo}
      liveConnected={liveConnected}
    >
      {page}
    </Shell>
  )
}
