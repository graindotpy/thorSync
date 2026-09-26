import { ArrowLeft, ArrowRight, Check, ExternalLink, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { Brand } from '../components/Brand'
import { ImportGames } from '../components/ImportGames'
import { HealthBadge } from '../components/StatusBadge'
import { completeOnboarding, configureSyncthingFolders, setWindowsGbaProfile } from '../lib/api'
import { fileSize } from '../lib/format'
import type { ArchiveUsage, DiagnosticCheck, Endpoint, WindowsGbaProfileId } from '../types'

interface OnboardingPageProps {
  endpoints: Endpoint[]
  demoMode: boolean
  navigate: (path: string) => void
  diagnostics?: DiagnosticCheck[]
  archive?: ArchiveUsage
  canExit?: boolean
}

const steps = ['Server check', 'Pair devices', 'Choose folders', 'Emulators', 'Inventory', 'Game names', 'Safety check']

export function OnboardingPage({ endpoints, demoMode, navigate, diagnostics, archive, canExit = false }: OnboardingPageProps) {
  const [step, setStep] = useState(0)
  const [busy, setBusy] = useState(false)
  const [hashing, setHashing] = useState(false)
  const [error, setError] = useState<string | null>(null)
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

  const last = step === steps.length - 1

  return (
    <div className="setup">
      <header className="masthead">
        <div className="masthead__inner">
          <Brand />
          {canExit && <button className="button button--ghost setup__leave" type="button" onClick={() => navigate('/')}>Leave setup</button>}
        </div>
      </header>

      <div className="setup__layout">
        <aside className="setup__contents">
          <h1>Setting up ThorSync</h1>
          <p>About five minutes. Nothing is overwritten while you’re here, and delivery stays off until the last step.</p>
          <ol>
            {steps.map((label, index) => (
              <li key={label} aria-current={index === step ? 'step' : undefined} className={index < step ? 'done' : ''}>
                <span className="setup__number">{index < step ? <Check size={13} strokeWidth={2.5} /> : String(index + 1).padStart(2, '0')}</span>
                {label}
              </li>
            ))}
          </ol>
        </aside>

        <section className="setup__panel">
          <div className="setup__progress" aria-hidden="true"><span style={{ width: `${((step + 1) / steps.length) * 100}%` }} /></div>
          <div className="setup__body">
            <p className="setup__step">Step {step + 1} of {steps.length}</p>
            {step === 0 && <ServerCheck diagnostics={diagnostics} archive={archive} />}
            {step === 1 && <PairDevices endpoints={endpoints} thorDeviceId={thorDeviceId} windowsDeviceId={windowsDeviceId} setThorDeviceId={setThorDeviceId} setWindowsDeviceId={setWindowsDeviceId} />}
            {step === 2 && <FolderSetup />}
            {step === 3 && <EmulatorProfiles windowsGbaProfile={windowsGbaProfile} setWindowsGbaProfile={setWindowsGbaProfileState} emulatorClosed={emulatorClosed} setEmulatorClosed={setEmulatorClosed} />}
            {step === 4 && <Inventory />}
            {step === 5 && <><StepHeading title="Give your saves game names" copy="Import a RetroArch playlist, or hash your ROMs here in the browser. You can skip this and do it later from the library." /><ImportGames demoMode={demoMode} onBusyChange={setHashing} /></>}
            {step === 6 && <SafetyCheck enableDelivery={enableDelivery} setEnableDelivery={setEnableDelivery} />}
            {error && <p className="form-error" role="alert">{error}</p>}
          </div>
          <footer className="setup__footer">
            <button className="button button--ghost" type="button" disabled={step === 0 || busy} onClick={() => { setError(null); setStep((value) => value - 1) }}><ArrowLeft size={16} />Back</button>
            <button className="button button--primary" type="button" disabled={busy || hashing} onClick={() => void goNext()}>
              {busy && <LoaderCircle size={16} className="spin" />}
              {last ? 'Finish setup' : 'Continue'}
              {!last && <ArrowRight size={16} />}
            </button>
          </footer>
        </section>
      </div>
    </div>
  )
}

function StepHeading({ title, copy }: { title: string; copy: string }) {
  return <div className="setup__heading"><h2>{title}</h2><p>{copy}</p></div>
}

function Checklist({ items }: { items: { label: string; value: string }[] }) {
  return (
    <ul className="checklist">
      {items.map((item) => <li key={item.label}><Check size={14} strokeWidth={2.5} aria-hidden="true" /><strong>{item.label}</strong><span>{item.value}</span></li>)}
    </ul>
  )
}

function ServerCheck({ diagnostics, archive }: { diagnostics?: DiagnosticCheck[]; archive?: ArchiveUsage }) {
  return (
    <>
      <StepHeading title="Check the ZimaOS hub" copy="ThorSync needs a writable archive and a working connection to the local Syncthing API." />
      {diagnostics?.length ? (
        <ul className="check-list">
          {diagnostics.map((check) => (
            <li className={`check check--${check.state}`} key={check.id}>
              <div className="check__copy"><strong>{check.name}</strong><p>{check.detail}</p></div>
              <div className="check__state"><HealthBadge state={check.state} /></div>
            </li>
          ))}
        </ul>
      ) : (
        <Checklist items={[
          { label: 'Metadata directory', value: 'Writable · SQLite WAL ready' },
          { label: 'Revision archive', value: 'Writable' },
          { label: 'Syncthing API', value: 'Connected · event stream ready' },
        ]} />
      )}
      {archive && <p className="help">{fileSize(archive.freeBytes)} free on the archive disk, with {fileSize(archive.reserveBytes)} held back as a safety reserve.</p>}
      <p className="aside-note">This step only reads configuration and storage. It doesn’t touch any save files.</p>
    </>
  )
}

function PairDevices({ endpoints, thorDeviceId, windowsDeviceId, setThorDeviceId, setWindowsDeviceId }: { endpoints: Endpoint[]; thorDeviceId: string; windowsDeviceId: string; setThorDeviceId: (value: string) => void; setWindowsDeviceId: (value: string) => void }) {
  const devices = endpoints.length ? endpoints : [{ id: 'thor', name: 'AYN Thor', status: 'offline' as const }, { id: 'windows', name: 'Gaming PC', status: 'offline' as const }]
  const openSyncthing = () => window.open(`${window.location.protocol}//${window.location.hostname}:8384`, '_blank', 'noopener,noreferrer')
  return (
    <>
      <StepHeading title="Connect both playing devices" copy="Pair each device directly with the Syncthing node on ZimaOS. The Thor and the PC never share a folder with each other." />
      <figure className="topology topology--compact" aria-label="Pairing layout">
        <div className="topology__node"><strong>AYN Thor</strong><small>BasicSync</small></div>
        <span className="topology__wire" aria-hidden="true" />
        <div className="topology__node topology__node--hub"><strong>ZimaOS hub</strong><small>Syncthing + ThorSync</small></div>
        <span className="topology__wire" aria-hidden="true" />
        <div className="topology__node"><strong>Gaming PC</strong><small>Syncthing</small></div>
      </figure>
      <div className="field-pair">
        <label className="field"><span>AYN Thor device ID</span><input value={thorDeviceId} onChange={(event) => setThorDeviceId(event.target.value)} placeholder="AAAAAAA-BBBBBBB-…" autoComplete="off" spellCheck={false} /></label>
        <label className="field"><span>Windows PC device ID</span><input value={windowsDeviceId} onChange={(event) => setWindowsDeviceId(event.target.value)} placeholder="CCCCCCC-DDDDDDD-…" autoComplete="off" spellCheck={false} /></label>
      </div>
      <ul className="device-status">
        {devices.map((device) => <li key={device.id}><strong>{device.name}</strong><span>{device.status === 'healthy' ? 'Paired and reachable' : 'Enter its Syncthing device ID above'}</span></li>)}
      </ul>
      <button type="button" className="button button--ghost" onClick={openSyncthing}>Open Syncthing<ExternalLink size={14} /></button>
    </>
  )
}

function FolderSetup() {
  return (
    <>
      <StepHeading title="Keep each endpoint separate" copy="ThorSync watches one hub folder per device, so .srm and .sav files with the same name never collide." />
      <table className="plain-table">
        <thead><tr><th scope="col">Device</th><th scope="col">Hub folder</th><th scope="col">On the device</th></tr></thead>
        <tbody>
          <tr><td>AYN Thor</td><td><code>/sync/thor</code></td><td>Emulation/Saves</td></tr>
          <tr><td>Windows PC</td><td><code>/sync/windows</code></td><td>Your emulator’s save folder</td></tr>
        </tbody>
      </table>
      <p className="aside-note">On Android, use ordinary shared storage. Private app folders and <code>Android/data</code> can’t be read reliably.</p>
    </>
  )
}

function EmulatorProfiles({ windowsGbaProfile, setWindowsGbaProfile, emulatorClosed, setEmulatorClosed }: { windowsGbaProfile: WindowsGbaProfileId; setWindowsGbaProfile: (value: WindowsGbaProfileId) => void; emulatorClosed: boolean; setEmulatorClosed: (value: boolean) => void }) {
  const windowsLabel = windowsGbaProfile === 'windows-mgba' ? 'Standalone mGBA (.sav + RTC)' : 'VBA-M (.sav)'
  return (
    <>
      <StepHeading title="Choose your Windows GBA emulator" copy="ThorSync keeps mGBA’s RTC data in history, and sends RetroArch only the raw battery bytes it understands." />
      <div className="choices" role="radiogroup" aria-label="Windows GBA emulator">
        <label className="choice"><input type="radio" checked={windowsGbaProfile === 'windows-mgba'} onChange={() => setWindowsGbaProfile('windows-mgba')} /><span><strong>Standalone mGBA <em>Recommended</em></strong><small>Raw and 16-byte RTC-wrapped saves are handled automatically.</small></span></label>
        <label className="choice"><input type="radio" checked={windowsGbaProfile === 'windows-vbam'} onChange={() => setWindowsGbaProfile('windows-vbam')} /><span><strong>VBA-M</strong><small>Raw battery saves only.</small></span></label>
      </div>
      <table className="plain-table">
        <thead><tr><th scope="col">System</th><th scope="col">AYN Thor</th><th scope="col">Windows</th></tr></thead>
        <tbody>
          <tr><td>GBA</td><td>RetroArch · mGBA (.srm)</td><td>{windowsLabel}</td></tr>
          <tr><td>NDS</td><td>RetroArch · melonDS (.srm)</td><td>melonDS (.sav)</td></tr>
        </tbody>
      </table>
      <label className="confirm-check">
        <input type="checkbox" checked={emulatorClosed} onChange={(event) => setEmulatorClosed(event.target.checked)} />
        <span><strong>I have closed all emulators</strong><small>ThorSync may re-check existing mapped saves straight away.</small></span>
      </label>
      <p className="aside-note">The original file and any opaque RTC bytes are archived before ThorSync writes either target format.</p>
    </>
  )
}

function Inventory() {
  return (
    <>
      <StepHeading title="Inventory existing saves" copy="A read-only scan. ThorSync archives every version it finds before asking which one should become the shared starting point." />
      <Checklist items={[
        { label: 'Identical files are stored once', value: 'Each device’s copy still keeps its own record of where it came from.' },
        { label: 'Unknown saves wait in Review', value: 'They’re archived and listed for you to match to a game. Nothing unknown is delivered.' },
      ]} />
    </>
  )
}

function SafetyCheck({ enableDelivery, setEnableDelivery }: { enableDelivery: boolean; setEnableDelivery: (value: boolean) => void }) {
  return (
    <>
      <StepHeading title="Ready to leave import mode" copy="From here, ThorSync only moves a mapped save once its emulator profile, extension and size all check out." />
      <Checklist items={[
        { label: 'Archive before write', value: 'Every delivery starts from an immutable revision' },
        { label: 'Per-device baselines', value: 'Tracked separately for each device' },
        { label: 'Write-loop protection', value: 'Idempotent operation journal ready' },
        { label: 'Quiet period', value: '5 seconds · two matching observations' },
      ]} />
      <label className="confirm-check">
        <input type="checkbox" checked={enableDelivery} onChange={(event) => setEnableDelivery(event.target.checked)} />
        <span><strong>Enable automatic delivery after setup</strong><small>Only straightforward updates move automatically. Diverging saves always pause.</small></span>
      </label>
    </>
  )
}
