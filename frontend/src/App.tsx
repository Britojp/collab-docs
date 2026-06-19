import { Routes, Route, Navigate } from 'react-router-dom'
import LoginPage from './pages/LoginPage'
import DocumentsPage from './pages/DocumentsPage'
import EditorPage from './pages/EditorPage'

function isAuthenticated() {
  return !!localStorage.getItem('token')
}

function PrivateRoute({ children }: { children: JSX.Element }) {
  return isAuthenticated() ? children : <Navigate to="/login" replace />
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/documents" element={
        <PrivateRoute><DocumentsPage /></PrivateRoute>
      } />
      <Route path="/documents/:id" element={
        <PrivateRoute><EditorPage /></PrivateRoute>
      } />
      <Route path="*" element={<Navigate to="/documents" replace />} />
    </Routes>
  )
}
