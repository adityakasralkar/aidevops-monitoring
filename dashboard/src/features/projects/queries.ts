import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { CreatedProject, Environment, Project } from '../../lib/types'

export const projectKeys = {
  all: ['projects'] as const,
  one: (id: string) => ['project', id] as const,
}

/** Your projects, newest first (the server's order). */
export function useProjects() {
  return useQuery({
    queryKey: projectKeys.all,
    queryFn: ({ signal }) => api<{ projects: Project[] }>('/projects', { signal }).then((r) => r.projects),
    refetchInterval: 30_000, // keeps "Receiving data" / "Last data" current
  })
}

/** One project; used by the sidebar to show which project is open. */
export function useProject(id: string | undefined) {
  return useQuery({
    queryKey: projectKeys.one(id ?? ''),
    queryFn: ({ signal }) => api<Project>(`/projects/${encodeURIComponent(id ?? '')}`, { signal }),
    enabled: Boolean(id),
  })
}

export interface NewProject {
  name: string
  environment: Environment
}

/** Creates a project. The response holds the API key, which is never shown again. */
export function useCreateProject() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: NewProject) => api<CreatedProject>('/projects', { method: 'POST', body: input }),
    // The result holds the API key: drop it from memory as soon as the popup closes.
    gcTime: 0,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectKeys.all }),
  })
}
