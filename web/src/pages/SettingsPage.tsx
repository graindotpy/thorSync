import { FileText, RotateCcw, Save } from 'lucide-react'
import { useState } from 'react'
import { ImportGames } from '../components/ImportGames'
import { Modal } from '../components/Modal'
import { PageHeader } from '../components/PageHeader'
import { setPropagation, setWindowsGbaProfile } from '../lib/api'
import { readThemePreference, saveThemePreference, type ThemePreference } from '../lib/theme'
import type { EmulatorSettings } from '../types'

interface SettingsPageProps {
  propagationEnabled: boolean
  quotaBytes: number
  emulatorSettings: EmulatorSettings
  onSaved: () => Promise<void>
  navigate?: (path: string) => void
  demoMode?: boolean
}

const themeOptions: { value: ThemePreference; label: string; detail: string }[] = [
  { value: 'system', label: 'Match this device', detail: 'Paper by day, ink at night.' },
  { value: 'paper', label: 'Paper', detail: 'Always light.' },
  { value: 'ink', label: 'Ink', detail: 'Always dark.' },
]

export function SettingsPage({ propagationEnabled, quotaBytes, emulatorSettings, onSaved, navigate, demoMode = false }: SettingsPageProps) {
  const [saved, setSaved] = useState(false)
  const [autoDelivery, setAutoDelivery] = useState(propagationEnabled)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [windowsGbaProfile, setWindowsGbaProfileState] = useState<'windows-mgba' | 'windows-vbam'>(emulatorSettings.windowsGbaProfileId)
  const [emulatorClosed, setEmulatorClosed] = useState(false)
  const [theme, setTheme] = useState<ThemePreference>(readThemePreference)
  const [importing, setImporting] = useState(false)
  const profileChanged = windowsGbaProfile !== emulatorSettings.windowsGbaProfileId || !emulatorSettings.configured
  const dirty = profileChanged || autoDelivery !== propagationEnabled

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

  const changeTheme = (value: ThemePreference) => {
    setTheme(value)
    saveThemePreference(value)
  }

  return (
    <>
      <PageHeader title="Settings" description="How ThorSync moves saves, and how this page looks on this device." />

      <div className="settings">
        <section className="setting-group" aria-labelledby="delivery-heading">
          <div className="setting-group__intro">
            <h2 id="delivery-heading">Delivery</h2>
            <p>When a checked save moves from one device to the other.</p>
          </div>
          <div className="setting-group__body">
            <label className="switch-row">
              <span><strong>Automatic delivery</strong><small>Deliver straightforward updates while no conflict is open. Diverging saves always pause, whatever this is set to.</small></span>
              <input type="checkbox" role="switch" className="switch" checked={autoDelivery} onChange={(event) => setAutoDelivery(event.target.checked)} />
            </label>
            <dl className="facts facts--settings">
              <div><dt>Quiet period before reading a save</dt><dd>5 seconds</dd></div>
              <div><dt>Archive soft quota</dt><dd>{(quotaBytes / 1024 ** 3).toFixed(0)} GiB <small>set in Docker</small></dd></div>
            </dl>
          </div>
        </section>

        <section className="setting-group" aria-labelledby="emulator-heading">
          <div className="setting-group__intro">
            <h2 id="emulator-heading">Windows GBA emulator</h2>
            <p>The format ThorSync writes GBA saves in for your PC.</p>
            {emulatorSettings.detectedProfileId === 'windows-mgba' && <p className="setting-group__detected">mGBA detected on this PC.</p>}
          </div>
          <div className="setting-group__body">
            {!emulatorSettings.configured && <p className="callout callout--attention callout--inline">One-time setup required. ThorSync found Windows GBA mappings but won’t rewrite them until you confirm the emulators are closed.</p>}
            <div className="choices" role="radiogroup" aria-label="Windows GBA emulator">
              <label className="choice"><input type="radio" name="windows-gba-profile" value="windows-mgba" checked={windowsGbaProfile === 'windows-mgba'} onChange={() => setWindowsGbaProfileState('windows-mgba')} /><span><strong>Standalone mGBA <em>Recommended</em></strong><small>Handles raw saves and mGBA’s 16-byte RTC data automatically.</small></span></label>
              <label className="choice"><input type="radio" name="windows-gba-profile" value="windows-vbam" checked={windowsGbaProfile === 'windows-vbam'} onChange={() => setWindowsGbaProfileState('windows-vbam')} /><span><strong>VBA-M</strong><small>Byte-for-byte raw battery saves, without an RTC wrapper.</small></span></label>
            </div>
            <p className="help">{emulatorSettings.affectedBindings} existing Windows GBA {emulatorSettings.affectedBindings === 1 ? 'mapping' : 'mappings'} will use this default. Individual games can keep a different profile from their own page.</p>
            {profileChanged && (
              <label className="confirm-check">
                <input type="checkbox" checked={emulatorClosed} onChange={(event) => setEmulatorClosed(event.target.checked)} />
                <span><strong>I have closed all emulators</strong><small>ThorSync will re-check existing saves as soon as this changes.</small></span>
              </label>
            )}
          </div>
        </section>

        <div className="save-bar" aria-live="polite">
          <span className={error ? 'save-bar__error' : ''}>{error ?? (saved ? 'Saved. New settings apply to the next delivery.' : dirty ? 'You have unsaved changes.' : 'Changes apply to future deliveries.')}</span>
          <button type="button" className="button button--primary" disabled={saving} onClick={() => void save()}><Save size={16} />{saving ? 'Saving…' : 'Save preferences'}</button>
        </div>

        <section className="setting-group" aria-labelledby="appearance-heading">
          <div className="setting-group__intro">
            <h2 id="appearance-heading">Appearance</h2>
            <p>Remembered on this device only. Applies straight away.</p>
          </div>
          <div className="setting-group__body">
            <div className="choices choices--row" role="radiogroup" aria-label="Theme">
              {themeOptions.map((option) => (
                <label className="choice" key={option.value}>
                  <input type="radio" name="theme" value={option.value} checked={theme === option.value} onChange={() => changeTheme(option.value)} />
                  <span><strong>{option.label}</strong><small>{option.detail}</small></span>
                </label>
              ))}
            </div>
          </div>
        </section>

        <section className="setting-group" aria-labelledby="library-heading">
          <div className="setting-group__intro">
            <h2 id="library-heading">Library and setup</h2>
            <p>Name more games, or walk through pairing and folders again.</p>
          </div>
          <div className="setting-group__body setting-group__body--actions">
            <button type="button" className="button" onClick={() => setImporting(true)}><FileText size={16} />Import game names</button>
            {navigate && <button type="button" className="button button--ghost" onClick={() => navigate('/onboarding')}><RotateCcw size={16} />Run setup again</button>}
          </div>
        </section>

        <section className="setting-group" aria-labelledby="access-heading">
          <div className="setting-group__intro">
            <h2 id="access-heading">Access</h2>
            <p>Set through Docker secrets, so it can’t be changed from here.</p>
          </div>
          <div className="setting-group__body">
            <dl className="facts facts--settings">
              <div><dt>Authentication</dt><dd>Cloudflare Access JWT</dd></div>
              <div><dt>Administrator</dt><dd>Configured via <code>THORSYNC_ADMIN_EMAIL_FILE</code></dd></div>
              <div><dt>Cross-origin requests</dt><dd>Disabled · same origin only</dd></div>
            </dl>
          </div>
        </section>
      </div>

      <Modal
        open={importing}
        wide
        title="Import game names"
        description="Match save files to games. Nothing here moves or changes a save."
        onClose={() => setImporting(false)}
        footer={<button type="button" className="button button--primary" onClick={() => setImporting(false)}>Done</button>}
      >
        <ImportGames demoMode={demoMode} onImported={() => void onSaved()} />
      </Modal>
    </>
  )
}
