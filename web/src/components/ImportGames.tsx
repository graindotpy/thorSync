import { CheckCircle2, FileText, LoaderCircle, LockKeyhole, Upload } from 'lucide-react'
import { useState, type ChangeEvent } from 'react'
import { importPlaylist, submitRomHashes, type RomHashRecord } from '../lib/api'

interface ImportGamesProps {
  demoMode: boolean
  onBusyChange?: (busy: boolean) => void
  onImported?: () => void
}

// Playlist import and in-browser ROM hashing. Shared by setup and the library.
export function ImportGames({ demoMode, onBusyChange, onImported }: ImportGamesProps) {
  const [playlistName, setPlaylistName] = useState<string | null>(null)
  const [romHashes, setRomHashes] = useState<RomHashRecord[]>([])
  const [hashing, setHashing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handlePlaylist = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    setPlaylistName(file.name)
    setError(null)
    if (!demoMode) {
      try {
        await importPlaylist(file)
        onImported?.()
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Playlist import failed')
      }
    }
  }

  const handleRoms = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    if (!files.length) return
    setHashing(true)
    onBusyChange?.(true)
    setError(null)
    try {
      const records = await Promise.all(files.map(hashRomLocally))
      setRomHashes(records)
      if (!demoMode) {
        await submitRomHashes(records)
        onImported?.()
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not hash the selected files')
    } finally {
      setHashing(false)
      onBusyChange?.(false)
      event.target.value = ''
    }
  }

  return (
    <div className="import-games">
      <div className="import-games__options">
        <label className="file-pick">
          <input type="file" accept=".lpl,application/json" onChange={(event) => void handlePlaylist(event)} />
          <span className="file-pick__icon">{playlistName ? <CheckCircle2 size={20} /> : <FileText size={20} />}</span>
          <span className="file-pick__copy">
            <strong>{playlistName ?? 'RetroArch playlist'}</strong>
            <small>{playlistName ? 'Sent for matching' : 'An .lpl file. Its paths and labels name your saves.'}</small>
          </span>
          <span className="file-pick__action"><Upload size={15} />Choose</span>
        </label>
        <label className="file-pick">
          <input type="file" accept=".gba,.nds" multiple onChange={(event) => void handleRoms(event)} />
          <span className="file-pick__icon">{hashing ? <LoaderCircle size={20} className="spin" /> : romHashes.length ? <CheckCircle2 size={20} /> : <FileText size={20} />}</span>
          <span className="file-pick__copy">
            <strong>{hashing ? 'Hashing locally…' : 'Local ROM hashes'}</strong>
            <small>{romHashes.length ? `${romHashes.length} hashes ready · no ROMs uploaded` : '.gba or .nds files, hashed in this browser.'}</small>
          </span>
          <span className="file-pick__action"><Upload size={15} />Choose</span>
        </label>
      </div>
      {romHashes.length > 0 && (
        <table className="hash-table">
          <thead><tr><th scope="col">File</th><th scope="col">SHA-1</th><th scope="col">CRC32</th></tr></thead>
          <tbody>
            {romHashes.slice(0, 4).map((record) => (
              <tr key={`${record.filename}-${record.sha1}`}><td>{record.filename}</td><td><code>{record.sha1.slice(0, 12)}…</code></td><td><code>{record.crc32}</code></td></tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="aside-note"><LockKeyhole size={15} />Only filenames, sizes and CRC32/SHA-1 values are sent, for matching against the licensed Libretro subset. ROM bytes never leave this device.</p>
      {error && <p className="form-error" role="alert">{error}</p>}
    </div>
  )
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
