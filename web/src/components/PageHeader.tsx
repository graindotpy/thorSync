import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: string
  description?: ReactNode
  actions?: ReactNode
  kicker?: ReactNode
}

export function PageHeader({ title, description, actions, kicker }: PageHeaderProps) {
  return (
    <header className="page-header">
      <div className="page-header__copy">
        {kicker && <div className="page-header__kicker">{kicker}</div>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="page-header__actions">{actions}</div>}
    </header>
  )
}

export function SectionHeader({ title, count, description, actions, id }: { title: string; count?: number; description?: ReactNode; actions?: ReactNode; id?: string }) {
  return (
    <div className="section-header">
      <div>
        <h2 id={id}>{title}{count !== undefined && <span className="section-header__count">{count}</span>}</h2>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="section-header__actions">{actions}</div>}
    </div>
  )
}
