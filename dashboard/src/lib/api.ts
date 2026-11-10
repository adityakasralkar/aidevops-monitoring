import { store } from '../store'
import { expired } from '../store/authSlice'
import type { ErrorBody } from './types'

/** Base address of the Go server, without a trailing slash. */
export const API_URL = (import.meta.env.VITE_API_URL || 'http://localhost:8080').replace(/\/+$/, '')

/** A failed request: the HTTP status plus the server's error code and message. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  /** Seconds to wait, from the Retry-After header (429 / 503). */
  readonly retryAfter?: number

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryAfter = retryAfter
  }
}

interface ApiOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
  /**
   * Which session token to send. Left out: the signed-in user's token, and a
   * 401 marks the session as expired. A string: that token, nothing else
   * happens on 401. null: no token (login, sign up).
   */
  token?: string | null
}

/**
 * Calls the API at /api/v1<path> and returns the JSON body (undefined for 204).
 * Throws ApiError for any non-2xx answer or when the server can't be reached.
 */
export async function api<T>(path: string, { method = 'GET', body, signal, token }: ApiOptions = {}): Promise<T> {
  const usesSession = token === undefined
  const bearer = usesSession ? store.getState().auth.token : token

  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (bearer) headers.Authorization = `Bearer ${bearer}`

  let res: Response
  try {
    res = await fetch(`${API_URL}/api/v1${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, 'network_error', "Can't reach the server. Check that it's running.")
  }

  if (res.status === 204) return undefined as T

  const data: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    const error = (data as Partial<ErrorBody> | null)?.error
    const retry = Number(res.headers.get('Retry-After'))
    if (res.status === 401 && usesSession && bearer) {
      // The token is no longer valid (expired or logged out elsewhere).
      store.dispatch(expired())
    }
    throw new ApiError(
      res.status,
      error?.code ?? 'http_error',
      error?.message ?? `Request failed (${res.status})`,
      Number.isFinite(retry) && retry > 0 ? retry : undefined,
    )
  }
  return data as T
}
