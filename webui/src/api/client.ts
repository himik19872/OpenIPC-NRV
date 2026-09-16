import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
})

// Интерсептор для JWT
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (r) => r,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export default api

// --- Типы ---

export interface Camera {
  id: string
  name: string
  rtsp_url: string
  main_stream?: string
  sub_stream?: string
  ip?: string
  mac?: string
  firmware?: string
  site_id?: string
  wg_ip?: string
  status: 'online' | 'offline' | 'recording'
  ptz?: boolean
  hw_info?: Record<string, any>
  settings?: Record<string, any>
  created_at: string
  updated_at: string
}

export interface CameraCommandResult {
  command: string
  output?: string
  success: boolean
  error?: string
}

export interface PTZStatus {
  pan: number
  tilt: number
  zoom: number
  supports_ptz: boolean
}

export interface PTZPreset {
  token: string
  name: string
}

export interface DiscoveredCamera {
  ip: string
  mac?: string
  firmware?: string
  model?: string
  main_stream: string
  sub_stream: string
  snapshot?: string
  online: boolean
}

export interface ScanResult {
  subnet: string
  total: number
  found: number
  cameras: DiscoveredCamera[]
}

export interface DetectionEvent {
  id: string
  camera_id: string
  camera_name?: string
  timestamp: string
  object_class: string
  confidence: number
  bbox?: { x: number; y: number; w: number; h: number }
  track_id?: number
  snapshot_path?: string
  thumbnail_path?: string
  metadata?: Record<string, any>
}

export interface Recording {
  id: string
  camera_id: string
  start_time: string
  end_time: string
  duration_sec: number
  file_path: string
  file_size: number
  resolution?: string
  codec?: string
  event_triggered: boolean
}

export interface ACSController {
  id: string
  name: string
  vendor: string
  ip: string
  port: number
  site_id?: string
  status: 'online' | 'offline'
  config?: Record<string, any>
  created_at: string
}

export interface ACSEvent {
  id: string
  controller_id: string
  door_id: string
  event_type: string
  card_number?: string
  user_id?: string
  timestamp: string
  camera_id?: string
  snapshot_path?: string
}

export interface Stats {
  total_cameras: number
  online_cameras: number
  total_events_24h: number
  disk_used_gb: number
  disk_total_gb: number
  acs_online: number
  acs_total: number
}

export interface StreamInfo {
  rtsp_url: string
  hls_url: string
  webrtc_url: string
  status: string
  main_hls_url: string
  sub_hls_url: string
  main_rtsp_url: string
  sub_rtsp_url: string
  snapshot_url: string
}

export interface PaginatedResponse<T> {
  events?: T[]
  recordings?: T[]
  total: number
  page: number
  page_size: number
}

// --- API методы ---

export const authAPI = {
  login: (username: string, password: string) =>
    api.post<{ token: string; expires_at: number; user: any }>('/auth/login', { username, password }),
}

export const camerasAPI = {
  list: () => api.get<Camera[]>('/cameras'),
  get: (id: string) => api.get<Camera>(`/cameras/${id}`),
  getStream: (id: string) => api.get<StreamInfo>(`/cameras/${id}/stream`),
  getSnapshot: (id: string) => api.get<{ message: string }>(`/cameras/${id}/snapshot`),
  create: (data: {
    name: string; rtsp_url?: string; main_stream?: string; sub_stream?: string;
    ip?: string; mac?: string; firmware?: string; username?: string; password?: string;
  }) => api.post<Camera>('/cameras', data),
  update: (id: string, data: {
    name?: string; rtsp_url?: string; main_stream?: string; sub_stream?: string;
    ip?: string; mac?: string; firmware?: string; status?: string;
    username?: string; password?: string; wg_ip?: string; ptz?: boolean;
  }) => api.patch<Camera>(`/cameras/${id}`, data),
  delete: (id: string) => api.delete(`/cameras/${id}`),
  // Перезапуск стримера камеры (служба Majestic на OpenIPC)
  restartStreamer: (id: string) =>
    api.post<CameraCommandResult>(`/cameras/${id}/restart-streamer`, {}),
  // Перезагрузка камеры целиком
  reboot: (id: string) =>
    api.post<CameraCommandResult>(`/cameras/${id}/reboot`, {}),
}

export const ptzAPI = {
  status: (id: string) => api.get<PTZStatus>(`/cameras/${id}/ptz/status`),
  move: (id: string, pan: number, tilt: number, zoom = 0, durationMs = 500) =>
    api.post<{ success: boolean }>(`/cameras/${id}/ptz/move`, {
      pan, tilt, zoom, duration_ms: durationMs,
    }),
  stop: (id: string) => api.post<{ success: boolean }>(`/cameras/${id}/ptz/stop`, {}),
  presets: (id: string) => api.get<PTZPreset[]>(`/cameras/${id}/ptz/presets`),
  gotoPreset: (id: string, token: string) =>
    api.post<{ success: boolean }>(`/cameras/${id}/ptz/presets/goto`, { token }),
}

export const scannerAPI = {
  scan: (subnet: string, username?: string, password?: string) =>
    api.post<ScanResult>('/scanner/scan', { subnet, username, password }),
  probe: (ip: string, username?: string, password?: string) =>
    api.post<DiscoveredCamera>('/scanner/probe', { ip, username, password }),
}
export const eventsAPI = {
  list: (params?: { camera_id?: string; page?: number; page_size?: number }) =>
    api.get<PaginatedResponse<DetectionEvent> & { events: DetectionEvent[] }>('/events', { params }),
  get: (id: string) => api.get<DetectionEvent>(`/events/${id}`),
}

export const recordingsAPI = {
  list: (params?: { camera_id?: string; page?: number; page_size?: number }) =>
    api.get<PaginatedResponse<Recording> & { recordings: Recording[] }>('/recordings', { params }),
  get: (id: string) => api.get<Recording>(`/recordings/${id}`),
  delete: (id: string) => api.delete(`/recordings/${id}`),
}

export const acsAPI = {
  listControllers: () => api.get<ACSController[]>('/acs/controllers'),
  getController: (id: string) => api.get<ACSController>(`/acs/controllers/${id}`),
  createController: (data: any) => api.post<ACSController>('/acs/controllers', data),
  deleteController: (id: string) => api.delete(`/acs/controllers/${id}`),
  listEvents: (params?: { page?: number; page_size?: number }) =>
    api.get<PaginatedResponse<ACSEvent>>('/acs/events', { params }),
  openDoor: (controllerID: string, doorID: string) =>
    api.post(`/acs/doors/${controllerID}/open`, { door_id: doorID }),
}

export const statsAPI = {
  get: () => api.get<Stats>('/stats'),
}