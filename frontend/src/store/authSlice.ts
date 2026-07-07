import { createSlice, createAsyncThunk, type PayloadAction } from '@reduxjs/toolkit';
import { authApi } from '../api/client';

interface User {
  id: string;
  username: string;
  email: string;
  full_name: string;
  role: string;
}

interface AuthState {
  token: string | null;
  user: User | null;
  loading: boolean;
  error: string | null;
}

const initialState: AuthState = {
  token: localStorage.getItem('access_token'),
  user: null,
  loading: false,
  error: null,
};

export const login = createAsyncThunk(
  'auth/login',
  async (creds: { username: string; password: string }) => {
    const { data } = await authApi.login(creds.username, creds.password);
    localStorage.setItem('access_token', data.access_token);
    localStorage.setItem('refresh_token', data.refresh_token);
    return data;
  },
);

export const fetchMe = createAsyncThunk('auth/me', async () => {
  const { data } = await authApi.me();
  return data;
});

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    logout(state) {
      state.token = null;
      state.user = null;
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
    },
    clearError(state) {
      state.error = null;
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.pending, (s) => { s.loading = true; s.error = null; })
      .addCase(login.fulfilled, (s, a: PayloadAction<{ access_token: string; user: User }>) => {
        s.loading = false;
        s.token = a.payload.access_token;
        s.user = a.payload.user;
      })
      .addCase(login.rejected, (s, a) => {
        s.loading = false;
        s.error = (a.error as Error).message || 'Ошибка входа';
      })
      .addCase(fetchMe.fulfilled, (s, a: PayloadAction<User>) => {
        s.user = a.payload;
      });
  },
});

export const { logout, clearError } = authSlice.actions;
export default authSlice.reducer;