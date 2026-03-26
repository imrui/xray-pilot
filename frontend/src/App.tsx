import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from '@/components/Layout'
import Login from '@/pages/Login'
import Users from '@/pages/Users'
import Groups from '@/pages/Groups'
import Nodes from '@/pages/Nodes'
import Profiles from '@/pages/Profiles'
import Logs from '@/pages/Logs'
import Settings from '@/pages/Settings'
import { useAuthStore } from '@/store/auth'
import { ConfirmProvider } from '@/components/ui/ConfirmProvider'

const queryClient = new QueryClient()

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token)
  return token ? <>{children}</> : <Navigate to="/login" replace />
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
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
              <Route index element={<Navigate to="/nodes" replace />} />
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
    </QueryClientProvider>
  )
}
