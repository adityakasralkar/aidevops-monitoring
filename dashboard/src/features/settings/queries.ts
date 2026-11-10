import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { CreatedProject, Project } from '../../lib/types'
import { projectKeys } from '../projects/queries'

const path = (id: string) => `/projects/${encodeURIComponent(id)}`

/** Renames the project; the sidebar and project list update afterwards. */
export function useRenameProject(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api<Project>(path(projectId), { method: 'PATCH', body: { name } }),
    onSuccess: (project) => {
      queryClient.setQueryData(projectKeys.one(projectId), project)
      void queryClient.invalidateQueries({ queryKey: projectKeys.all })
    },
  })
}

/** Replaces the API key. The old key stops working at once; the new one is shown only now. */
export function useRotateKey(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => api<CreatedProject>(`${path(projectId)}/api-key`, { method: 'POST' }),
    gcTime: 0, // the result holds the new key: forget it as soon as the popup closes
    onSuccess: ({ api_key: _key, ...project }) => {
      queryClient.setQueryData(projectKeys.one(projectId), project)
      void queryClient.invalidateQueries({ queryKey: projectKeys.all })
    },
  })
}

/** Deletes the project and everything stored for it. */
export function useDeleteProject(projectId: string) {
  return useMutation({
    mutationFn: () => api<void>(path(projectId), { method: 'DELETE' }),
  })
}
