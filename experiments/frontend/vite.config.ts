import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Proxy /serviceregistry/* to the core so the browser never makes a
// cross-origin request. The core does not serve CORS headers (and must
// not be modified), so the proxy is the correct solution here.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/serviceregistry': 'http://localhost:8080',
    },
  },
})
