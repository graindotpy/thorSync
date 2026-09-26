import { PageHeader } from '../components/PageHeader'
import type { Conflict, EmulatorProfile, Game, UnassignedFile } from '../types'
import { ConflictsPage } from './ConflictsPage'
import { UnassignedPage } from './UnassignedPage'

interface ReviewPageProps {
  conflicts: Conflict[]
  files: UnassignedFile[]
  games: Game[]
  profiles: EmulatorProfile[]
  demoMode: boolean
  navigate: (path: string) => void
  onRefresh: () => Promise<void>
}

export function ReviewPage({ conflicts, files, games, profiles, demoMode, navigate, onRefresh }: ReviewPageProps) {
  const waiting = conflicts.length + files.length
  return (
    <>
      <PageHeader
        title="Review"
        description={waiting
          ? 'ThorSync stops and asks when it can’t be sure. Nothing listed here reaches a device until you decide.'
          : 'Nothing is waiting on you. ThorSync stops and asks here whenever it can’t be sure.'}
      />
      <ConflictsPage conflicts={conflicts} demoMode={demoMode} navigate={navigate} />
      <UnassignedPage files={files} games={games} profiles={profiles} onMapped={onRefresh} navigate={navigate} demoMode={demoMode} />
    </>
  )
}
