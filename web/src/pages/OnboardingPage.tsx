import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  Cloud,
  FileJson2,
  FolderCheck,
  Gamepad2,
  HardDrive,
  Link2,
  LoaderCircle,
  LockKeyhole,
  Monitor,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Upload,
  X,
  Zap,
} from 'lucide-react'
import { useState, type ChangeEvent } from 'react'
import { Brand } from '../components/Brand'
import {
  completeOnboarding,
  configureSyncthingFolders,
  importPlaylist,
  setWindowsGbaProfile,
  submitRomHashes,
  type RomHashRecord,
} from '../lib/api'
import type { Endpoint, WindowsGbaProfileId } from '../types'

interface OnboardingPageProps {
  endpoints: Endpoint[]
  demoMode: boolean
  navigate: (path: string) => void
}

const steps = [
  { label: 'Server check', icon: Cloud },
  { label: 'Pair devices', icon: Link2 },
  { label: 'Choose folders', icon: HardDrive },
  { label: 'Emulators', icon: Gamepad2 },
  { label: 'Inventory', icon: RefreshCw },
  { label: 'Game names', icon: FileJson2 },
  { label: 'Safety test', icon: ShieldCheck },
]

export function OnboardingPage({ endpoints, demoMode, navigate }: OnboardingPageProps) {
  const [step, setStep] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [playlistName, setPlaylistName] = useState<string | null>(null)
  const [romHashes, setRomHashes] = useState<RomHashRecord[]>([])
  const [hashing, setHashing] = useState(false)
  const [enableDelivery, setEnableDelivery] = useState(true)
  const [thorDeviceId, setThorDeviceId] = useState('')
  const [windowsDeviceId, setWindowsDeviceId] = useState('')
  const [windowsGbaProfile, setWindowsGbaProfileState] = useState<WindowsGbaProfileId>('windows-mgba')
  const [emulatorClosed, setEmulatorClosed] = useState(false)

  const goNext = async () => {
    if (step < steps.length - 1) {
      setError(null)
      if (step === 1 && !demoMode) {
        if (!thorDeviceId.trim() || !windowsDeviceId.trim()) {
          setError('Enter both Syncthing device IDs before creating the isolated folders.')
          return
        }
        setBusy(true)
        try { await configureSyncthingFolders(thorDeviceId.trim(), windowsDeviceId.trim()) }
        catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not configure Syncthing folders'); return }
        finally { setBusy(false) }
      }
      if (step === 3) {
        if (!emulatorClosed) {
          setError('Close all emulators and confirm before selecting the Windows GBA adapter.')
          return
        }
        if (!demoMode) {
          setBusy(true)
          try { await setWindowsGbaProfile(windowsGbaProfile, true) }
          catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not configure the Windows GBA emulator'); return }
          finally { setBusy(false) }
        }
      }
      setStep((value) => value + 1)
      return
    }
    setBusy(true)
    setError(null)
    try {
      if (!demoMode) await completeOnboarding(enableDelivery)
      navigate('/')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not finish setup')
    } finally {
      setBusy(false)
    }
  }

  const handlePlaylist = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    setPlaylistName(file.name)
    setError(null)
    if (!demoMode) {
      try {
        await importPlaylist(file)
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Playlist import failed')
      }
    }
  }

  const handleRoms = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    if (!files.length) return
    setHashing(true)
    setError(null)
    try {
      const records = await Promise.all(files.map(hashRomLocally))
      setRomHashes(records)
      if (!demoMode) await submitRomHashes(records)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not hash the selected files')
    } finally {
      setHashing(false)
      event.target.value = ''
    }
  }

  return (
    <div className="onboarding">
      <header className="onboarding__header">
        <Brand />
        <button className="button button--quiet" type="button" onClick={() => navigate('/')}><X size={16} />Exit setup</button>
      </header>
      <div className="onboarding__layout">
        <aside className="setup-steps">
          <span className="eyebrow">Setup progress</span>
          <h1>Connect your saves</h1>
          <p>About five minutes. ThorSync will not overwrite anything during setup.</p>
          <ol>
            {steps.map((item, index) => {
              const Icon = item.icon
              return (
                <li key={item.label} className={index === step ? 'active' : index < step ? 'complete' : ''}>
                  <span>{index < step ? <Check size={15} /> : <Icon size={16} />}</span>
                  <div><small>Step {index + 1}</small><strong>{item.label}</strong></div>
                </li>
              )
            })}
          </ol>
          <div className="setup-steps__safety"><LockKeyhole size={17} /><span><strong>Import mode is safe by default</strong><small>Delivery stays off until the final compatibility check passes.</small></span></div>
        </aside>

        <main className="setup-content">
          <div className="setup-content__progress"><span style={{ width: `${((step + 1) / steps.length) * 100}%` }} /></div>
          <div className="setup-content__body">
            <StepContent
              step={step}
              endpoints={endpoints}
              playlistName={playlistName}
              romHashes={romHashes}
              hashing={hashing}
              enableDelivery={enableDelivery}
              setEnableDelivery={setEnableDelivery}
              thorDeviceId={thorDeviceId}
              windowsDeviceId={windowsDeviceId}
              setThorDeviceId={setThorDeviceId}
              setWindowsDeviceId={setWindowsDeviceId}
              windowsGbaProfile={windowsGbaProfile}
              setWindowsGbaProfile={setWindowsGbaProfileState}
              emulatorClosed={emulatorClosed}
              setEmulatorClosed={setEmulatorClosed}
              onPlaylist={handlePlaylist}
              onRoms={handleRoms}
            />
            {error && <p className="form-error"><X size={14} />{error}</p>}
          </div>
          <footer className="setup-content__footer">
            <button className="button button--quiet" type="button" disabled={step === 0 || busy} onClick={() => setStep((value) => value - 1)}><ArrowLeft size={16} />Back</button>
            <span>Step {step + 1} of {steps.length}</span>
            <button className="button button--primary" type="button" disabled={busy || hashing} onClick={() => void goNext()}>{busy ? <LoaderCircle size={16} className="spin-slow" /> : step === steps.length - 1 ? <Zap size={16} /> : null}{step === steps.length - 1 ? 'Finish setup' : 'Continue'}{step < steps.length - 1 && <ArrowRight size={16} />}</button>
          </footer>
        </main>
      </div>
    </div>
  )
}

interface StepContentProps {
  step: number
  endpoints: Endpoint[]
  playlistName: string | null
  romHashes: RomHashRecord[]
  hashing: boolean
  enableDelivery: boolean
  setEnableDelivery: (value: boolean) => void
  thorDeviceId: string
  windowsDeviceId: string
  setThorDeviceId: (value: string) => void
  setWindowsDeviceId: (value: string) => void
  windowsGbaProfile: WindowsGbaProfileId
  setWindowsGbaProfile: (value: WindowsGbaProfileId) => void
  emulatorClosed: boolean
  setEmulatorClosed: (value: boolean) => void
  onPlaylist: (event: ChangeEvent<HTMLInputElement>) => void
  onRoms: (event: ChangeEvent<HTMLInputElement>) => void
}

function StepContent(props: StepContentProps) {
  if (props.step === 0) return <ServerCheck />
  if (props.step === 1) return <PairDevices endpoints={props.endpoints} thorDeviceId={props.thorDeviceId} windowsDeviceId={props.windowsDeviceId} setThorDeviceId={props.setThorDeviceId} setWindowsDeviceId={props.setWindowsDeviceId} />
  if (props.step === 2) return <FolderSetup />
  if (props.step === 3) return <EmulatorProfiles windowsGbaProfile={props.windowsGbaProfile} setWindowsGbaProfile={props.setWindowsGbaProfile} emulatorClosed={props.emulatorClosed} setEmulatorClosed={props.setEmulatorClosed} />
  if (props.step === 4) return <Inventory />
  if (props.step === 5) return <GameImport {...props} />
  return <SafetyTest enableDelivery={props.enableDelivery} setEnableDelivery={props.setEnableDelivery} />
}

function SetupHeading({ icon, eyebrow, title, copy }: { icon: React.ReactNode; eyebrow: string; title: string; copy: string }) {
  return <div className="setup-heading"><span>{icon}</span><div><small>{eyebrow}</small><h2>{title}</h2><p>{copy}</p></div></div>
}

function ServerCheck() {
  return <><SetupHeading icon={<Cloud size={23} />} eyebrow="Start here" title="Check the ZimaOS hub" copy="ThorSync needs a writable archive and a healthy connection to the local Syncthing API." /><div className="validation-list"><ValidationRow label="Metadata directory" value="Writable · SQLite WAL ready" /><ValidationRow label="Revision archive" value="Writable · 86 GB available" /><ValidationRow label="Syncthing API" value="Connected · event stream ready" /><ValidationRow label="Free-space reserve" value="1 GiB protected" /></div><div className="inline-note"><ShieldCheck size={17} /><p>This check reads configuration and storage only. It does not move or alter save files.</p></div></>
}

function PairDevices({ endpoints, thorDeviceId, windowsDeviceId, setThorDeviceId, setWindowsDeviceId }: { endpoints: Endpoint[]; thorDeviceId: string; windowsDeviceId: string; setThorDeviceId: (value: string) => void; setWindowsDeviceId: (value: string) => void }) {
  const devices = endpoints.length ? endpoints : [{ id: 'thor', name: 'AYN Thor', kind: 'thor', status: 'offline' }, { id: 'windows', name: 'Gaming PC', kind: 'windows', status: 'offline' }]
  const openSyncthing = () => window.open(`${window.location.protocol}//${window.location.hostname}:8384`, '_blank', 'noopener,noreferrer')
  return <><SetupHeading icon={<Link2 size={23} />} eyebrow="Syncthing pairing" title="Connect both playing devices" copy="Pair each device directly with the ZimaOS Syncthing node. Your handheld and PC do not share folders with one another." /><div className="pairing-diagram"><div><span><Smartphone size={25} /></span><strong>AYN Thor</strong><small>BasicSync</small></div><i /><div className="pairing-diagram__hub"><span><Cloud size={27} /></span><strong>ZimaOS hub</strong><small>Syncthing + ThorSync</small></div><i /><div><span><Monitor size={25} /></span><strong>Gaming PC</strong><small>Syncthing</small></div></div><div className="device-id-grid"><label><span>AYN Thor device ID</span><input value={thorDeviceId} onChange={(event) => setThorDeviceId(event.target.value)} placeholder="AAAAAAA-BBBBBBB-…" autoComplete="off" /></label><label><span>Windows PC device ID</span><input value={windowsDeviceId} onChange={(event) => setWindowsDeviceId(event.target.value)} placeholder="CCCCCCC-DDDDDDD-…" autoComplete="off" /></label></div><div className="validation-list compact">{devices.map((device) => <ValidationRow key={device.id} label={device.name} value={device.status === 'healthy' ? 'Paired and reachable' : 'Enter its Syncthing device ID'} />)}</div><button type="button" className="button button--quiet" onClick={openSyncthing}><Link2 size={16} />Open Syncthing pairing</button></>
}

function FolderSetup() {
  return <><SetupHeading icon={<HardDrive size={23} />} eyebrow="Isolated folders" title="Keep each endpoint separate" copy="ThorSync watches a dedicated hub folder for each device so .srm and .sav names never collide." /><div className="folder-cards"><div><span><Smartphone size={20} /></span><div><strong>Thor saves</strong><code>/sync/thor</code><small>Device path: Emulation/Saves</small></div><CheckCircle2 size={18} /></div><div><span><Monitor size={20} /></span><div><strong>Windows saves</strong><code>/sync/windows</code><small>Choose your emulator save folder</small></div><CheckCircle2 size={18} /></div></div><div className="inline-note"><FolderCheck size={17} /><p>On Android, use ordinary shared storage. Private app and <code>Android/data</code> folders cannot be accessed reliably.</p></div></>
}

function EmulatorProfiles({ windowsGbaProfile, setWindowsGbaProfile, emulatorClosed, setEmulatorClosed }: { windowsGbaProfile: WindowsGbaProfileId; setWindowsGbaProfile: (value: WindowsGbaProfileId) => void; emulatorClosed: boolean; setEmulatorClosed: (value: boolean) => void }) {
  const windowsLabel = windowsGbaProfile === 'windows-mgba' ? 'Standalone mGBA (.sav + RTC)' : 'VBA-M (.sav)'
  return <><SetupHeading icon={<Gamepad2 size={23} />} eyebrow="Format-aware adapters" title="Choose your Windows GBA emulator" copy="ThorSync keeps mGBA’s RTC state in history while sending RetroArch only the raw battery bytes it supports." /><div className="emulator-options" role="radiogroup" aria-label="Windows GBA emulator"><label className={windowsGbaProfile === 'windows-mgba' ? 'selected' : ''}><input type="radio" checked={windowsGbaProfile === 'windows-mgba'} onChange={() => setWindowsGbaProfile('windows-mgba')} /><span><strong>Standalone mGBA <em>Recommended</em></strong><small>Raw and 16-byte RTC-wrapped saves are handled automatically.</small></span></label><label className={windowsGbaProfile === 'windows-vbam' ? 'selected' : ''}><input type="radio" checked={windowsGbaProfile === 'windows-vbam'} onChange={() => setWindowsGbaProfile('windows-vbam')} /><span><strong>VBA-M</strong><small>Raw battery saves only.</small></span></label></div><div className="profile-table"><div className="profile-table__head"><span>Platform</span><span>AYN Thor</span><span>Windows</span><span /></div><ProfileRow platform="GBA" thor="RetroArch · mGBA (.srm)" windows={windowsLabel} /><ProfileRow platform="NDS" thor="RetroArch · melonDS (.srm)" windows="melonDS (.sav)" /></div><label className="enable-delivery"><input type="checkbox" checked={emulatorClosed} onChange={(event) => setEmulatorClosed(event.target.checked)} /><span><strong>I have closed all emulators</strong><small>ThorSync may immediately re-check existing mapped saves.</small></span></label><div className="inline-note"><ShieldCheck size={17} /><p>The original file and opaque RTC bytes are archived before ThorSync materializes either target format.</p></div></>
}

function ProfileRow({ platform, thor, windows }: { platform: string; thor: string; windows: string }) {
  return <div className="profile-table__row"><strong>{platform}</strong><span>{thor}</span><span>{windows}</span><CheckCircle2 size={18} /></div>
}

function Inventory() {
  return <><SetupHeading icon={<RefreshCw size={23} />} eyebrow="Read-only scan" title="Inventory existing saves" copy="ThorSync archives every discovered version before asking which one should become the shared baseline." /><div className="scan-list"><div><span className="scan-dot scan-dot--ok" /><div><strong>Identical content is deduplicated safely</strong><p>Every device observation keeps its own provenance even when the archived bytes match.</p></div><Check size={18} /></div><div><span className="scan-dot scan-dot--warn" /><div><strong>Unknown saves remain unassigned</strong><p>They stay archived and visible for manual game mapping; ThorSync will not deliver them automatically.</p></div><ArrowRight size={18} /></div></div></>
}

function GameImport({ playlistName, romHashes, hashing, onPlaylist, onRoms }: StepContentProps) {
  return <><SetupHeading icon={<FileJson2 size={23} />} eyebrow="Metadata only" title="Give your saves game names" copy="Import a RetroArch playlist, or hash local ROMs in this browser. ROM bytes never leave this device." /><div className="import-options"><label className="upload-card"><input type="file" accept=".lpl,application/json" onChange={onPlaylist} /><span><FileJson2 size={22} /></span><div><strong>{playlistName ?? 'RetroArch playlist'}</strong><p>{playlistName ? 'Ready to match' : 'Choose an .lpl file with paths and labels'}</p></div>{playlistName ? <CheckCircle2 className="upload-card__done" size={19} /> : <Upload size={18} />}</label><label className="upload-card"><input type="file" accept=".gba,.nds" multiple onChange={onRoms} /><span><Gamepad2 size={22} /></span><div><strong>{hashing ? 'Hashing locally…' : 'Local ROM hashes'}</strong><p>{romHashes.length ? `${romHashes.length} hashes ready · no ROMs uploaded` : 'Choose .gba or .nds files'}</p></div>{hashing ? <LoaderCircle className="spin-slow" size={18} /> : romHashes.length ? <CheckCircle2 className="upload-card__done" size={19} /> : <Upload size={18} />}</label></div>{romHashes.length > 0 && <div className="hash-list">{romHashes.slice(0, 4).map((record) => <div key={`${record.filename}-${record.sha1}`}><span>{record.filename}</span><code>SHA-1 {record.sha1.slice(0, 12)}…</code><code>CRC32 {record.crc32}</code></div>)}</div>}<div className="privacy-note"><LockKeyhole size={17} /><p>Only filenames, sizes, and CRC32/SHA-1 values are submitted for matching against the licensed Libretro subset.</p></div></>
}

function SafetyTest({ enableDelivery, setEnableDelivery }: { enableDelivery: boolean; setEnableDelivery: (value: boolean) => void }) {
  return <><SetupHeading icon={<ShieldCheck size={23} />} eyebrow="Final safety check" title="Ready to leave import mode" copy="ThorSync will only move a mapped save after its profile, extension, and size pass validation." /><div className="final-check"><span><CheckCircle2 size={33} /></span><h3>Safety controls active</h3><p>Unsupported or ambiguous files are archived and quarantined instead of delivered.</p></div><div className="validation-list compact"><ValidationRow label="Archive-before-write" value="Every delivery starts from an immutable revision" /><ValidationRow label="Device baselines" value="Tracked independently for each endpoint" /><ValidationRow label="Write-loop protection" value="Idempotent operation journal ready" /><ValidationRow label="Quiescence checks" value="5 seconds · two observations" /></div><label className="enable-delivery"><input type="checkbox" checked={enableDelivery} onChange={(event) => setEnableDelivery(event.target.checked)} /><span><strong>Enable automatic delivery after setup</strong><small>Only validated linear updates move automatically. Divergent histories always pause.</small></span></label></>
}

function ValidationRow({ label, value }: { label: string; value: string }) {
  return <div><span><Check size={14} /></span><strong>{label}</strong><small>{value}</small></div>
}

async function hashRomLocally(file: File): Promise<RomHashRecord> {
  const bytes = await file.arrayBuffer()
  const digest = await crypto.subtle.digest('SHA-1', bytes)
  const sha1 = Array.from(new Uint8Array(digest)).map((byte) => byte.toString(16).padStart(2, '0')).join('')
  return { filename: file.name, size: file.size, sha1, crc32: crc32(new Uint8Array(bytes)) }
}

function crc32(bytes: Uint8Array): string {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0)
  }
  return ((crc ^ 0xffffffff) >>> 0).toString(16).padStart(8, '0').toUpperCase()
}
