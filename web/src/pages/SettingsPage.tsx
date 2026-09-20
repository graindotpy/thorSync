import { Bell, CloudCog, LockKeyhole, Save, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { setPropagation } from '../lib/api'

export function SettingsPage({ propagationEnabled, quotaBytes, onSaved }: { propagationEnabled: boolean; quotaBytes: number; onSaved: () => Promise<void> }) {
  const [saved, setSaved] = useState(false)
  const [autoDelivery, setAutoDelivery] = useState(propagationEnabled)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const save = async () => {
    setSaving(true); setError(null)
    try { await setPropagation(autoDelivery); await onSaved(); setSaved(true); window.setTimeout(() => setSaved(false), 3000) }
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
        <section className="panel settings-section settings-section--locked">
          <div className="settings-section__heading"><span><LockKeyhole size={20} /></span><div><h2>Access policy</h2><p>Security settings are provided through Docker secrets.</p></div><span className="locked-badge"><ShieldCheck size={13} />Protected</span></div>
          <dl className="security-list"><div><dt>Authentication</dt><dd>Cloudflare Access JWT</dd></div><div><dt>Administrator</dt><dd>Configured via <code>THORSYNC_ADMIN_EMAIL_FILE</code></dd></div><div><dt>CORS</dt><dd>Disabled · same origin only</dd></div></dl>
        </section>
      </div>
      <div className="settings-save"><span>{error ?? (saved ? 'Broker preferences saved.' : 'Changes apply to future broker operations.')}</span><button type="button" className="button button--primary" disabled={saving} onClick={() => void save()}><Save size={16} />{saving ? 'Saving…' : 'Save preferences'}</button></div>
    </>
  )
}
