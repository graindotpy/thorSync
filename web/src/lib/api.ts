import {
  getMockGameDetail,
  mockActivity,
  mockArchive,
  mockConflicts,
  mockDiagnostics,
  mockEndpoints,
  mockGames,
  mockProfiles,
} from '../data/mock'
import type {
  ActivityItem,
  ArchiveUsage,
  Conflict,
  DashboardData,
  DiagnosticCheck,
  Endpoint,
  EmulatorSettings,
  EmulatorProfile,
  Game,
  GameDetail,
  OnboardingStatus,
  PromoteRequest,
  RestoreRequest,
  ServerEvent,
  UnassignedFile,
} from '../types'

const API_ROOT = '/api/v1'
const REQUEST_TIMEOUT_MS = 5_000
const DEMO_ENABLED = import.meta.env.DEV || import.meta.env.VITE_ENABLE_DEMO === 'true'

type Envelope<T> = T | { data: T }

function unwrap<T>(value: Envelope<T>): T {
  if (value && typeof value === 'object' && 'data' in value) {
    return value.data
  }
  return value as T
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
  const headers = new Headers(init?.headers)
  headers.set('Accept', 'application/json')
  if (init?.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  try {
    const response = await fetch(`${API_ROOT}${path}`, {
      ...init,
      credentials: 'same-origin',
      headers,
      signal: controller.signal,
    })

    if (!response.ok) {
      const message = await response.text().catch(() => '')
      throw new Error(message || `Request failed (${response.status})`)
    }

    return unwrap<T>((await response.json()) as Envelope<T>)
  } finally {
    window.clearTimeout(timeout)
  }
}

const fallbackOnboarding: OnboardingStatus = {
  complete: true,
  currentStep: 6,
  syncthingConnected: true,
  storageWritable: true,
  endpointsConfigured: true,
  inventoryComplete: true,
  propagationEnabled: true,
}

const fallbackEmulatorSettings: EmulatorSettings = {
  windowsGbaProfileId: 'windows-mgba',
  configured: true,
  affectedBindings: 0,
}

export async function loadDashboard(): Promise<{ data: DashboardData; isDemo: boolean }> {
  try {
    const [games, endpoints, activity, conflicts, diagnostics, archive, onboarding, unassigned, emulatorSettings, profiles] =
      await Promise.all([
        request<Game[]>('/games'),
        request<Endpoint[]>('/endpoints'),
        request<ActivityItem[]>('/activity?limit=40'),
        request<Conflict[]>('/conflicts?state=open'),
        request<DiagnosticCheck[]>('/diagnostics'),
        request<ArchiveUsage>('/archive/usage'),
        request<OnboardingStatus>('/onboarding'),
        request<UnassignedFile[]>('/unassigned'),
        request<EmulatorSettings>('/settings/emulators'),
        request<EmulatorProfile[]>('/profiles'),
      ])

    return {
      data: { games, endpoints, activity, conflicts, diagnostics, archive, onboarding, unassigned, emulatorSettings, profiles },
      isDemo: false,
    }
  } catch (error) {
    if (!DEMO_ENABLED) throw error
    return {
      data: {
        games: mockGames,
        endpoints: mockEndpoints,
        activity: mockActivity,
        conflicts: mockConflicts,
        diagnostics: mockDiagnostics,
        archive: mockArchive,
        onboarding: fallbackOnboarding,
        unassigned: [],
        emulatorSettings: fallbackEmulatorSettings,
        profiles: mockProfiles,
      },
      isDemo: true,
    }
  }
}

export async function loadGame(gameId: string): Promise<{ data: GameDetail; isDemo: boolean }> {
  try {
    return { data: await request<GameDetail>(`/games/${encodeURIComponent(gameId)}`), isDemo: false }
  } catch (error) {
    if (!DEMO_ENABLED) throw error
    const game = getMockGameDetail(gameId)
    if (!game) throw new Error('Game not found')
    return { data: game, isDemo: true }
  }
}

export async function restoreRevision(gameId: string, payload: RestoreRequest): Promise<void> {
  await request(`/games/${encodeURIComponent(gameId)}/restore`, {
    method: 'POST',
    headers: { 'Idempotency-Key': payload.idempotencyKey },
    body: JSON.stringify(payload),
  })
}

export async function createManualSnapshot(gameId: string, expectedHeadId: string): Promise<void> {
  const idempotencyKey = createIdempotencyKey()
  await request(`/games/${encodeURIComponent(gameId)}/snapshot`, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ expectedHeadId, idempotencyKey }),
  })
}

export async function promoteConflict(
  conflictId: string,
  payload: PromoteRequest,
): Promise<void> {
  await request(`/conflicts/${encodeURIComponent(conflictId)}/promote`, {
    method: 'POST',
    headers: { 'Idempotency-Key': payload.idempotencyKey },
    body: JSON.stringify(payload),
  })
}

export async function completeOnboarding(enableDelivery: boolean): Promise<void> {
  await request('/onboarding/complete', {
    method: 'POST',
    body: JSON.stringify({ enableDelivery }),
  })
}

export async function importPlaylist(file: File): Promise<void> {
  const formData = new FormData()
  formData.append('playlist', file)
  await request('/imports/retroarch', { method: 'POST', body: formData })
}

export interface RomHashRecord {
  filename: string
  size: number
  crc32: string
  sha1: string
}

export async function submitRomHashes(records: RomHashRecord[]): Promise<void> {
  await request('/imports/rom-hashes', {
    method: 'POST',
    body: JSON.stringify({ records }),
  })
}

export async function mapSaveToGame(
  gameId: string,
  endpointId: 'thor' | 'windows',
  profileId: string,
  relativePath: string,
  emulatorClosed = false,
): Promise<void> {
  await request(`/games/${encodeURIComponent(gameId)}/bindings`, {
    method: 'PUT',
    body: JSON.stringify({ endpointId, profileId, relativePath, emulatorClosed }),
  })
}

export async function configureSyncthingFolders(thorDeviceId: string, windowsDeviceId: string): Promise<void> {
  await request('/onboarding/folders', {
    method: 'POST',
    body: JSON.stringify({ thorDeviceId, windowsDeviceId }),
  })
}

export async function runEndpointAction(endpointId: string, action: 'scan' | 'pause' | 'resume'): Promise<void> {
  await request(`/endpoints/${encodeURIComponent(endpointId)}/${action}`, { method: 'POST', body: '{}' })
}

export async function setPropagation(enabled: boolean): Promise<void> {
  await request('/settings/propagation', { method: 'POST', body: JSON.stringify({ enabled }) })
}

export async function setWindowsGbaProfile(
  profileId: 'windows-mgba' | 'windows-vbam',
  emulatorClosed: boolean,
  applyToExisting = true,
): Promise<void> {
  await request('/settings/emulators/windows-gba', {
    method: 'PUT',
    body: JSON.stringify({ profileId, emulatorClosed, applyToExisting }),
  })
}

export async function uploadArtwork(gameId: string, file: File): Promise<void> {
  const formData = new FormData()
  formData.append('artwork', file)
  await request(`/games/${encodeURIComponent(gameId)}/artwork`, { method: 'POST', body: formData })
}

export function subscribeToEvents(
  onEvent: (event: ServerEvent) => void,
  onConnectionChange: (connected: boolean) => void,
): () => void {
  if (typeof EventSource === 'undefined') return () => undefined

  const source = new EventSource(`${API_ROOT}/events`, { withCredentials: true })
  source.onopen = () => onConnectionChange(true)
  source.onerror = () => onConnectionChange(false)
  source.onmessage = (message) => {
    try {
      onEvent(JSON.parse(message.data) as ServerEvent)
    } catch {
      // Ignore malformed events. The next reconciliation refresh remains authoritative.
    }
  }

  return () => source.close()
}

export function createIdempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `thorsync-${Date.now()}-${Math.random()}`
}

export function isMockGame(game: Game): boolean {
  return mockGames.some((candidate) => candidate.id === game.id)
}
