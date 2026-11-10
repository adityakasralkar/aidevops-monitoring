import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { Service } from '../../lib/types'

/**
 * Programs that sent data to this project in the last 24 hours.
 * `poll` re-checks every 5 seconds, so a newly connected app shows up by itself.
 */
export function useServices(projectId: string, { poll = false } = {}) {
  return useQuery({
    queryKey: ['services', projectId, '24h'],
    queryFn: ({ signal }) =>
      api<{ services: Service[] }>(`/projects/${encodeURIComponent(projectId)}/services?range=24h`, { signal }).then(
        (r) => r.services,
      ),
    refetchInterval: poll ? 5_000 : false,
  })
}

const languageNames: Record<string, string> = {
  nodejs: 'Node.js',
  python: 'Python',
  java: 'Java',
  go: 'Go',
  dotnet: '.NET',
  php: 'PHP',
  ruby: 'Ruby',
  rust: 'Rust',
  cpp: 'C++',
  erlang: 'Erlang',
  swift: 'Swift',
  webjs: 'Browser JS',
}

/** "nodejs" → "Node.js"; unknown values are shown as they are. */
export function languageName(value: string | undefined): string {
  if (!value) return 'Unknown'
  return languageNames[value.toLowerCase()] ?? value
}
