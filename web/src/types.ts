export type Platform = 'GBA' | 'NDS'
export type DeviceKind = 'thor' | 'windows' | 'server'
export type HealthState = 'healthy' | 'degraded' | 'offline'
export type DeliveryState = 'delivered' | 'syncing' | 'paused' | 'missing'
export type Provenance = 'confirmed' | 'inferred' | 'unknown'
export type WindowsGbaProfileId = 'windows-mgba' | 'windows-vbam'

export interface Endpoint {
  id: string
  name: string
  kind: DeviceKind
  profile: string
  folder: string
  status: HealthState
  lastSeenAt: string | null
  pendingFiles: number
  version?: string
}

export interface Revision {
  id: string
  gameId: string
  shortHash: string
  sourceDeviceId: string
  sourceDeviceName: string
  sourceModifiedAt: string
  observedAt: string
  size: number
  provenance: Provenance
  kind: 'capture' | 'restore' | 'promotion' | 'snapshot'
  state: 'head' | 'history' | 'branch' | 'quarantined'
  note?: string
}

export interface Game {
  id: string
  title: string
  platform: Platform
  saveName: string
  emulator: string
  updatedAt: string
  sourceDeviceId: string
  sourceDeviceName: string
  deliveryState: DeliveryState
  currentRevisionId: string
  revisionCount: number
  hasConflict: boolean
  healthMessage?: string
  playTimeLabel?: string
  accent: string
  artworkUrl?: string
}

export interface GameDetail extends Game {
  revisions: Revision[]
  bindings: SaveBinding[]
}

export interface SaveBinding {
  id: string
  endpointId: string
  endpointName: string
  relativePath: string
  extension: '.sav' | '.srm'
  baselineRevisionId: string | null
  deliveryState: DeliveryState
  lastDeliveredAt: string | null
  profileId: string
  profileName: string
  format: string
  hasRtc: boolean
}

export interface EmulatorSettings {
  windowsGbaProfileId: WindowsGbaProfileId
  configured: boolean
  detectedProfileId?: WindowsGbaProfileId
  affectedBindings: number
}

export interface EmulatorProfile {
  id: string
  name: string
  endpointId: 'thor' | 'windows'
  platform: Platform | Lowercase<Platform>
  extension: string
  format: string
}

export interface ActivityItem {
  id: string
  type: 'capture' | 'delivery' | 'conflict' | 'restore' | 'missing' | 'system'
  title: string
  detail: string
  occurredAt: string
  gameId?: string
  gameTitle?: string
  deviceName?: string
  tone: 'positive' | 'warning' | 'neutral' | 'danger'
}

export interface Conflict {
  id: string
  gameId: string
  gameTitle: string
  platform: Platform
  openedAt: string
  reason: string
  currentHead: Revision
  branch: Revision
}

export interface DiagnosticCheck {
  id: string
  name: string
  detail: string
  state: HealthState
  checkedAt: string
  latencyMs?: number
}

export interface ArchiveUsage {
  usedBytes: number
  quotaBytes: number
  freeBytes: number
  reserveBytes: number
  blobCount: number
  revisionCount: number
}

export interface OnboardingStatus {
  complete: boolean
  currentStep: number
  syncthingConnected: boolean
  storageWritable: boolean
  endpointsConfigured: boolean
  inventoryComplete: boolean
  propagationEnabled: boolean
}

export interface UnassignedFile {
  id: string
  endpointId: 'thor' | 'windows'
  relativePath: string
  blobHash?: string
  size: number
  sourceModifiedAt: string | null
  observedAt: string
  provenance: Provenance
  state: string
  detail?: string
  suggestedProfileId?: string
  compatibleProfileIds: string[]
  reviewOnly?: boolean
}

export interface DashboardData {
  games: Game[]
  endpoints: Endpoint[]
  activity: ActivityItem[]
  conflicts: Conflict[]
  diagnostics: DiagnosticCheck[]
  archive: ArchiveUsage
  onboarding: OnboardingStatus
  unassigned: UnassignedFile[]
  emulatorSettings: EmulatorSettings
  profiles: EmulatorProfile[]
}

export interface RestoreRequest {
  revisionId: string
  expectedHeadId: string
  emulatorClosed: true
  idempotencyKey: string
}

export interface PromoteRequest {
  revisionId: string
  expectedHeadId: string
  emulatorClosed: true
  idempotencyKey: string
}

export interface ServerEvent {
  type: 'game.updated' | 'delivery.updated' | 'conflict.opened' | 'health.updated'
  gameId?: string
  occurredAt: string
}
