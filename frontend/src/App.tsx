import { Routes, Route, Navigate } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from './store';
import MainLayout from './layouts/MainLayout';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import CamerasPage from './pages/CamerasPage';
import CameraDetailPage from './pages/CameraDetailPage';
import LiveViewPage from './pages/LiveViewPage';
import RecordingsPage from './pages/RecordingsPage';
import NetworkScannerPage from './pages/NetworkScannerPage';
import UsersPage from './pages/UsersPage';
import SettingsPage from './pages/SettingsPage';

function App() {
  const token = useSelector((s: RootState) => s.auth.token);

  if (!token) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }

  return (
    <MainLayout>
      <Routes>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/cameras" element={<CamerasPage />} />
        <Route path="/cameras/:id" element={<CameraDetailPage />} />
        <Route path="/live" element={<LiveViewPage />} />
        <Route path="/recordings" element={<RecordingsPage />} />
        <Route path="/scanner" element={<NetworkScannerPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </MainLayout>
  );
}

export default App;