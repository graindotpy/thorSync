export function BrandMark({ size = 22 }: { size?: number }) {
  // Two endpoints and the hub between them: the shape of every ThorSync delivery.
  return (
    <svg className="brand__mark" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 12h16" stroke="currentColor" strokeWidth="1.6" />
      <circle cx="4" cy="12" r="2.6" fill="var(--paper, #f3eee4)" stroke="currentColor" strokeWidth="1.6" />
      <circle cx="20" cy="12" r="2.6" fill="var(--paper, #f3eee4)" stroke="currentColor" strokeWidth="1.6" />
      <circle cx="12" cy="12" r="4.2" fill="currentColor" />
    </svg>
  )
}

export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <span className="brand" aria-label="ThorSync">
      <BrandMark />
      {!compact && <span className="brand__wordmark" aria-hidden="true">ThorSync</span>}
    </span>
  )
}
