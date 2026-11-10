import { QueryClient } from '@tanstack/react-query'
import { ApiError } from './api'

// One client for the whole app, with the defaults every query starts from.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000, // data younger than 15 s is shown without refetching
      // Retry once for server or network trouble; a 4xx answer won't change.
      retry: (failures, error) => failures < 1 && !(error instanceof ApiError && error.status >= 400 && error.status < 500),
      refetchOnWindowFocus: false,
    },
  },
})
