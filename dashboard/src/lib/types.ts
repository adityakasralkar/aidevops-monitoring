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

export type Environment = 'live' | 'test'

/** A project as listed by GET /api/v1/projects and GET /api/v1/projects/{id}. */
export interface Project {
  id: string
  name: string
  environment: Environment
  key_prefix: string
  created_at: string
  key_created_at: string
  /** null until the first data arrives with this project's key. */
  last_used_at: string | null
}

/** POST /api/v1/projects: the project plus its API key, returned only this once. */
export interface CreatedProject extends Project {
  api_key: string
}
