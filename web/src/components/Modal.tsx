import { X } from 'lucide-react'
import { useEffect, useRef, type ReactNode } from 'react'

interface ModalProps {
  open: boolean
  title: string
  description?: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}

export function Modal({ open, title, description, onClose, children, footer, wide = false }: ModalProps) {
  const dialogRef = useRef<HTMLElement>(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose

  useEffect(() => {
    if (!open) return
    const previous = document.activeElement as HTMLElement | null
    dialogRef.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeRef.current()
    }
    document.addEventListener('keydown', onKey)
    document.body.classList.add('modal-open')
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.classList.remove('modal-open')
      previous?.focus?.()
    }
  }, [open])

  if (!open) return null

  return (
    <div className="modal-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section ref={dialogRef} tabIndex={-1} className={`modal ${wide ? 'modal--wide' : ''}`} role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <header className="modal__header">
          <div>
            <h2 id="modal-title">{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <button type="button" className="icon-button" aria-label="Close" onClick={onClose}>
            <X size={18} />
          </button>
        </header>
        <div className="modal__content">{children}</div>
        {footer && <footer className="modal__footer">{footer}</footer>}
      </section>
    </div>
  )
}

export function ConfirmCheck({ checked, onChange, title, detail }: { checked: boolean; onChange: (value: boolean) => void; title: string; detail: string }) {
  return (
    <label className="confirm-check">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span>
        <strong>{title}</strong>
        <small>{detail}</small>
      </span>
    </label>
  )
}
