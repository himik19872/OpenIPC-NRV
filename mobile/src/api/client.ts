import axios from 'axios';
import * as SecureStore from 'expo-secure-store';

const API_BASE = 'http://192.168.1.10:8000/api';  // замени на свой IP/домен

const api = axios.create({
  baseURL: API_BASE,
  headers: { 'Content-Type': 'application/json' },
});

// Интерцептор: токен
api.interceptors.request.use(async (config) => {
  const token = await SecureStore.getItemAsync('access_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

export default api;

export const authApi = {
  login: (username: string, password: string) =>
    api.post('/auth/login', { username, password }),
};

export const camerasApi = {
  list: () => api.get('/cameras'),
  get: (id: string) => api.get(`/cameras/${id}`),
  startRecording: (cameraId: string) =>
    api.post(`/cameras/${cameraId}/recordings/start`),
  stopRecording: (cameraId: string, recordingId: string) =>
    api.post(`/cameras/${cameraId}/recordings/stop`, { recording_id: recordingId }),
  getRecordings: (cameraId: string) =>
    api.get(`/cameras/${cameraId}/recordings`),
};