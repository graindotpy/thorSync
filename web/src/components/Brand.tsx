import { Check, Cloud, RefreshCw } from 'lucide-react'

export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`brand ${compact ? 'brand--compact' : ''}`} aria-label="ThorSync">
      <span className="brand__mark" aria-hidden="true">
        <Cloud size={21} strokeWidth={2.2} />
        <RefreshCw className="brand__orbit" size={11} strokeWidth={3} />
        <Check className="brand__check" size={8} strokeWidth={3.5} />
      </span>
      {!compact && (
        <span className="brand__wordmark">
          Thor<span>Sync</span>
        </span>
      )}
    </div>
  )
}
