import { Bell, CloudCog, Gamepad2, LockKeyhole, Save, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { setPropagation, setWindowsGbaProfile } from '../lib/api'
import type { EmulatorSettings } from '../types'

export function SettingsPage({ propagationEnabled, quotaBytes, emulatorSettings, onSaved }: { propagationEnabled: boolean; quotaBytes: number; emulatorSettings: EmulatorSettings; onSaved: () => Promise<void> }) {
  const [saved, setSaved] = useState(false)
  const [autoDelivery, setAutoDelivery] = useState(propagationEnabled)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [windowsGbaProfile, setWindowsGbaProfileState] = useState<'windows-mgba' | 'windows-vbam'>(emulatorSettings.windowsGbaProfileId)
  const [emulatorClosed, setEmulatorClosed] = useState(false)
  const profileChanged = windowsGbaProfile !== emulatorSettings.windowsGbaProfileId || !emulatorSettings.configured
  const save = async () => {
    if (profileChanged && !emulatorClosed) {
      setError('Close mGBA, VBA-M, and RetroArch, then confirm below before changing the save adapter.')
      return
    }
    setSaving(true); setError(null)
    try {
      if (profileChanged) await setWindowsGbaProfile(windowsGbaProfile, true)
      await setPropagation(autoDelivery)
      await onSaved()
      setSaved(true)
      window.setTimeout(() => setSaved(false), 3000)
    }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not save broker settings') }
    finally { setSaving(false) }
  }
  return (
    <>
      <PageHeader eyebrow="Server preferences" title="Settings" description="Safe defaults for capture, delivery, capacity, and notifications." />
      <div className="settings-layout">
        <section className="panel settings-section">
          <div className="settings-section__heading"><span><CloudCog size={20} /></span><div><h2>Save broker</h2><p>Control when validated revisions move between endpoints.</p></div></div>
          <label className="toggle-row"><span><strong>Automatic delivery</strong><small>Deliver linear updates when no conflict is open.</small></span><input type="checkbox" role="switch" checked={autoDelivery} onChange={(event) => setAutoDelivery(event.target.checked)} /></label>
          <label className="field-row"><span><strong>Quiescence window</strong><small>Wait before reading a changed save.</small></span><div className="input-suffix"><input value="5" readOnly /><span>seconds</span></div></label>
          <label className="field-row"><span><strong>Archive soft quota</strong><small>Configured through Docker; warnings appear at 80% and 90%.</small></span><div className="input-suffix"><input value={(quotaBytes / 1024 ** 3).toFixed(0)} readOnly /><span>GiB</span></div></label>
        </section>
        <section className="panel settings-section">
          <div className="settings-section__heading"><span><Bell size={20} /></span><div><h2>Attention signals</h2><p>Choose what appears as an urgent status in the library.</p></div></div>
          <label className="toggle-row"><span><strong>Conflicts</strong><small>Two saves diverged from their shared baseline.</small></span><input type="checkbox" role="switch" defaultChecked /></label>
          <label className="toggle-row"><span><strong>Missing device copies</strong><small>A save disappeared from a paired device.</small></span><input type="checkbox" role="switch" defaultChecked /></label>
          <label className="toggle-row"><span><strong>Capacity warnings</strong><small>The archive approaches its soft limit or reserve.</small></span><input type="checkbox" role="switch" defaultChecked /></label>
        </section>
        <section className="panel settings-section settings-section--wide">
          <div className="settings-section__heading"><span><Gamepad2 size={20} /></span><div><h2>Windows GBA emulator</h2><p>Choose how ThorSync materializes GBA saves for your PC.</p></div>{emulatorSettings.detectedProfileId === 'windows-mgba' && <span className="locked-badge"><ShieldCheck size={13} />mGBA detected</span>}</div>
          {!emulatorSettings.configured && <div className="inline-note"><ShieldCheck size={17} /><p>One-time setup required. ThorSync found Windows GBA mappings but will not rewrite them until you confirm the emulators are closed.</p></div>}
          <div className="emulator-options" role="radiogroup" aria-label="Windows GBA emulator">
            <label className={windowsGbaProfile === 'windows-mgba' ? 'selected' : ''}><input type="radio" name="windows-gba-profile" value="windows-mgba" checked={windowsGbaProfile === 'windows-mgba'} onChange={() => setWindowsGbaProfileState('windows-mgba')} /><span><strong>Standalone mGBA <em>Recommended</em></strong><small>Handles raw saves and mGBA’s 16-byte RTC data automatically.</small></span></label>
            <label className={windowsGbaProfile === 'windows-vbam' ? 'selected' : ''}><input type="radio" name="windows-gba-profile" value="windows-vbam" checked={windowsGbaProfile === 'windows-vbam'} onChange={() => setWindowsGbaProfileState('windows-vbam')} /><span><strong>VBA-M</strong><small>Uses byte-for-byte raw battery saves without an RTC wrapper.</small></span></label>
          </div>
          <p className="settings-help">{emulatorSettings.affectedBindings} existing Windows GBA {emulatorSettings.affectedBindings === 1 ? 'mapping' : 'mappings'} will use this default. Individual games can retain a different profile.</p>
          {profileChanged && <label className="enable-delivery"><input type="checkbox" checked={emulatorClosed} onChange={(event) => setEmulatorClosed(event.target.checked)} /><span><strong>I have closed all emulators</strong><small>ThorSync will re-check existing saves immediately after this change.</small></span></label>}
        </section>
        <section className="panel settings-section settings-section--locked">
          <div className="settings-section__heading"><span><LockKeyhole size={20} /></span><div><h2>Access policy</h2><p>Security settings are provided through Docker secrets.</p></div><span className="locked-badge"><ShieldCheck size={13} />Protected</span></div>
          <dl className="security-list"><div><dt>Authentication</dt><dd>Cloudflare Access JWT</dd></div><div><dt>Administrator</dt><dd>Configured via <code>THORSYNC_ADMIN_EMAIL_FILE</code></dd></div><div><dt>CORS</dt><dd>Disabled · same origin only</dd></div></dl>
        </section>
      </div>
      <div className="settings-save"><span>{error ?? (saved ? 'Broker preferences saved.' : 'Changes apply to future broker operations.')}</span><button type="button" className="button button--primary" disabled={saving} onClick={() => void save()}><Save size={16} />{saving ? 'Saving…' : 'Save preferences'}</button></div>
    </>
  )
}
