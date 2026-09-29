import type {
  ListRecommendationsResponse,
  LoginResponse,
  PendingSelectionDTO,
  RecommendationDTO,
  SelectRequest,
  UserInfo,
} from './types'

const SESSION_PATH = '/api/v1/session'

/** Sends the browser to the login page, returning here afterwards. */
export function redirectToLogin(): void {
  const next = window.location.pathname + window.location.search
  window.location.assign(`/login?next=${encodeURIComponent(next)}`)
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (res.status === 401 && !url.startsWith(SESSION_PATH)) {
    // Session missing or expired (e.g. the admin password changed).
    redirectToLogin()
  }
  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new Error(`${res.status} ${res.statusText}${body ? `: ${body}` : ''}`)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return res.json() as Promise<T>
}

function postJSON<T>(url: string, body: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

function recommendationPath(namespace: string, vpaName: string, containerName: string): string {
  return `/api/v1/recommendations/${encodeURIComponent(namespace)}/${encodeURIComponent(vpaName)}/${encodeURIComponent(containerName)}`
}

export function listRecommendations(): Promise<ListRecommendationsResponse> {
  return request<ListRecommendationsResponse>('/api/v1/recommendations')
}

export function getRecommendation(
  namespace: string,
  vpaName: string,
  containerName: string,
): Promise<RecommendationDTO> {
  return request<RecommendationDTO>(recommendationPath(namespace, vpaName, containerName))
}

/**
 * Queues the current recommendation's requested resource(s) as a pending
 * selection for this project to write back later -- it does not touch Git
 * itself. Rejected (400) if a requested resource isn't configured or
 * individually eligible.
 */
export function selectRecommendation(
  namespace: string,
  vpaName: string,
  containerName: string,
  selection: SelectRequest,
): Promise<PendingSelectionDTO> {
  return postJSON<PendingSelectionDTO>(`${recommendationPath(namespace, vpaName, containerName)}/select`, selection)
}

/** Exchanges the admin credentials for a session cookie. */
export function login(username: string, password: string): Promise<LoginResponse> {
  return postJSON<LoginResponse>(SESSION_PATH, { username, password })
}

export function logout(): Promise<void> {
  return request<void>(SESSION_PATH, { method: 'DELETE' })
}

export function getUserInfo(): Promise<UserInfo> {
  return request<UserInfo>(`${SESSION_PATH}/userinfo`)
}
