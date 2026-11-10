// API response types. They mirror the Go structs in server/internal/api.

/** GET /api/v1/auth/me and the `user` field of a login response. */
export interface User {
  id: string
  email: string
  created_at: string
}

/** POST /api/v1/auth/login */
export interface LoginResponse {
  token: string
  token_type: 'Bearer'
  expires_at: string
  user: User
}

/** Every error response: { "error": { "code": "...", "message": "..." } } */
export interface ErrorBody {
  error: { code: string; message: string }
}
