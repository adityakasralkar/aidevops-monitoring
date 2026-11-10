// API response types. They mirror the Go structs in server/internal/api.

/** GET /api/v1/auth/me and the `user` field of a login response. */
export interface User {
  id: string
  email: string
  created_at: string
}
