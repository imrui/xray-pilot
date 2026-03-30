import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from '@/components/Layout'
import Dashboard from '@/pages/Dashboard'
import Login from '@/pages/Login'
import Users from '@/pages/Users'
import Groups from '@/pages/Groups'
import Nodes from '@/pages/Nodes'
import Profiles from '@/pages/Profiles'
import Logs from '@/pages/Logs'
import Settings from '@/pages/Settings'
import { useAuthStore } from '@/store/auth'
import { ConfirmProvider } from '@/components/ui/ConfirmProvider'
import { GlobalToastProvider } from '@/components/ui/GlobalToastProvider'

const queryClient = new QueryClient()

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token)
  return token ? <>{children}</> : <Navigate to="/login" replace />
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <GlobalToastProvider>
        <ConfirmProvider>
          <BrowserRouter>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route
                path="/"
                element={
                  <ProtectedRoute>
                    <Layout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Navigate to="/dashboard" replace />} />
                <Route path="dashboard" element={<Dashboard />} />
                <Route path="users" element={<Users />} />
                <Route path="groups" element={<Groups />} />
                <Route path="nodes" element={<Nodes />} />
                <Route path="profiles" element={<Profiles />} />
                <Route path="logs" element={<Logs />} />
                <Route path="settings" element={<Settings />} />
              </Route>
            </Routes>
          </BrowserRouter>
        </ConfirmProvider>
      </GlobalToastProvider>
    </QueryClientProvider>
  )
}
