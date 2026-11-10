import { QueryClient } from '@tanstack/react-query'

// One client for the whole app, with the defaults every query starts from.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000, // data younger than 15 s is shown without refetching
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})
