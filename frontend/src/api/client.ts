import axios from 'axios';

const API_BASE = '/api';

const api = axios.create({
  baseURL: API_BASE,
  headers: { 'Content-Type': 'application/json' },
});

// Интерцептор: добавляем токен
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Интерцептор: при 401 пробуем обновить токен
api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config;
    if (error.response?.status === 401 && !original._retry) {
      original._retry = true;
      const refresh = localStorage.getItem('refresh_token');
      if (refresh) {
        try {
          const { data } = await axios.post(`${API_BASE}/auth/refresh`, { refresh_token: refresh });
          localStorage.setItem('access_token', data.access_token);
          localStorage.setItem('refresh_token', data.refresh_token);
          original.headers.Authorization = `Bearer ${data.access_token}`;
          return api(original);
        } catch {
          localStorage.clear();
          window.location.href = '/login';
        }
      }
    }
    return Promise.reject(error);
  },
);

export default api;

// ---- API функции ----

export const authApi = {
  login: (username: string, password: string) =>
    api.post('/auth/login', { username, password }),
  register: (data: { username: string; email: string; password: string; full_name: string }) =>
    api.post('/auth/register', data),
  me: () => api.get('/auth/me'),
  refresh: (refresh_token: string) =>
    api.post('/auth/refresh', { refresh_token }),
};

export const camerasApi = {
  list: (params?: { skip?: number; limit?: number; is_enabled?: boolean }) =>
    api.get('/cameras', { params }),
  get: (id: string) => api.get(`/cameras/${id}`),
  create: (data: Record<string, unknown>) => api.post('/cameras', data),
  update: (id: string, data: Record<string, unknown>) => api.put(`/cameras/${id}`, data),
  delete: (id: string) => api.delete(`/cameras/${id}`),
  status: (id: string) => api.get(`/cameras/${id}/status`),
  startRecording: (cameraId: string) =>
    api.post(`/cameras/${cameraId}/recordings/start`),
  stopRecording: (cameraId: string, recordingId: string) =>
    api.post(`/cameras/${cameraId}/recordings/stop`, { recording_id: recordingId }),
  getRecordings: (cameraId: string, params?: { skip?: number; limit?: number }) =>
    api.get(`/cameras/${cameraId}/recordings`, { params }),
  getEvents: (cameraId: string, params?: { skip?: number; limit?: number }) =>
    api.get(`/cameras/${cameraId}/events`, { params }),

  // ---- OpenIPC Majestic API ----
  openipc: {
    detail: (id: string) => api.get(`/cameras/${id}/openipc/detail`),
    snapshot: (id: string) => api.get(`/cameras/${id}/openipc/snapshot`),
    nightOn: (id: string) => api.post(`/cameras/${id}/openipc/night/on`),
    nightOff: (id: string) => api.post(`/cameras/${id}/openipc/night/off`),
    nightToggle: (id: string) => api.post(`/cameras/${id}/openipc/night/toggle`),
    ircut: (id: string) => api.post(`/cameras/${id}/openipc/ircut`),
    light: (id: string) => api.post(`/cameras/${id}/openipc/light`),
    config: (id: string) => api.get(`/cameras/${id}/openipc/config`),
    metrics: (id: string) => api.get(`/cameras/${id}/openipc/metrics`),
    // Audio / Intercom
    audioStreamUrl: (id: string, codec = 'opus') => api.get(`/cameras/${id}/openipc/audio/stream-url`, { params: { codec } }),
    audioOutputToggle: (id: string, enable: boolean) => api.post(`/cameras/${id}/openipc/audio/output-toggle`, null, { params: { enable } }),
    // SIP
    sipGet: (id: string) => api.get(`/cameras/${id}/openipc/sip`),
    sipConfig: (id: string, data: Record<string, unknown>) => api.post(`/cameras/${id}/openipc/sip`, null, { params: data }),
    // Motion
    motionGet: (id: string) => api.get(`/cameras/${id}/openipc/motion`),
    motionSet: (id: string, data: Record<string, unknown>) => api.post(`/cameras/${id}/openipc/motion`, null, { params: data }),
    // HLS
    hlsToggle: (id: string, enabled: boolean) => api.post(`/cameras/${id}/openipc/hls`, null, { params: { enabled } }),
    // ONVIF
    onvifConfig: (id: string, data: Record<string, unknown>) => api.post(`/cameras/${id}/openipc/onvif`, null, { params: data }),
    // Outgoing
    outgoingConfig: (id: string, data: Record<string, unknown>) => api.post(`/cameras/${id}/openipc/outgoing`, null, { params: data }),
    // Patch config
    patchConfig: (id: string, partial: Record<string, unknown>) => api.patch(`/cameras/${id}/openipc/config`, partial),
  },
};

export const usersApi = {
  list: (params?: { skip?: number; limit?: number }) =>
    api.get('/users', { params }),
  get: (id: string) => api.get(`/users/${id}`),
  update: (id: string, data: Record<string, unknown>) => api.put(`/users/${id}`, data),
  delete: (id: string) => api.delete(`/users/${id}`),
};

export const systemApi = {
  health: () => api.get('/health'),
  stats: () => api.get('/stats'),
};

export const scannerApi = {
  getSubnet: () => api.get('/scanner/subnet'),
  scan: (subnet?: string) => api.post('/scanner/scan', null, { params: { subnet } }),
  checkIp: (ip: string) => api.get(`/scanner/check/${ip}`),
  addFound: (ip: string) => api.post(`/scanner/add-found/${ip}`),
};