import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useAuth } from './hooks/useApi'
import Layout from './components/Layout'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import CamerasPage from './pages/CamerasPage'
import CameraDetailPage from './pages/CameraDetailPage'
import ScannerPage from './pages/ScannerPage'
import EventsPage from './pages/EventsPage'
import RecordingsPage from './pages/RecordingsPage'
import ACSPage from './pages/ACSPage'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth()
  const location = useLocation()

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/cameras" element={<CamerasPage />} />
                <Route path="/cameras/:id" element={<CameraDetailPage />} />
                <Route path="/scanner" element={<ScannerPage />} />
                <Route path="/events" element={<EventsPage />} />
                <Route path="/recordings" element={<RecordingsPage />} />
                <Route path="/acs" element={<ACSPage />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  )
}