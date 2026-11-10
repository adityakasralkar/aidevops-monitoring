import { useMatches } from '@tanstack/react-router'
import { useEffect } from 'react'
import { PageHeader } from '../components/layout/PageHeader'

/**
 * Stand-in for a screen that is not built yet: shows only the page title,
 * which comes from the route's staticData.
 */
export function PlaceholderPage() {
  const title = useMatches({ select: (matches) => matches[matches.length - 1]?.staticData.title }) ?? 'Page'

  useEffect(() => {
    document.title = `${title} · AIDevOps`
  }, [title])

  return <PageHeader title={title} />
}
