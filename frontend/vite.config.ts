import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/auth':      'http://localhost:8080',
      '/documents': 'http://localhost:8080',
      '/metrics':   'http://localhost:8080',
      '/ws':        { target: 'ws://localhost:8080', ws: true },
    },
  },
})
