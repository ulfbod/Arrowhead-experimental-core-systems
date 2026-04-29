import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development, proxy each core system to its default port.
// In production, build the dashboard and serve it from the Go binary.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/serviceregistry': 'http://localhost:8080',
      '/health':          'http://localhost:8080',
      '/authentication':  'http://localhost:8081',
      '/authorization':   'http://localhost:8082',
      '/orchestration':   'http://localhost:8083', // default: dynamic
    },
  },
  build: {
    outDir: 'dist',
  },
})
