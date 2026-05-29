import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development, proxy each core system to its default port.
// More-specific paths must come before less-specific ones.
// In production, build the dashboard and serve it from the Go binary.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/serviceregistry':                 'http://localhost:8080',
      '/health':                          'http://localhost:8080',
      '/authentication':                  'http://localhost:8081',
      '/consumerauthorization':            'http://localhost:8082',
      '/serviceorchestration/orchestration/flexiblestore': 'http://localhost:8085',
      '/serviceorchestration/orchestration/simplestore':   'http://localhost:8084',
      '/serviceorchestration/orchestration/pull':          'http://localhost:8083',
      // Virtual dev-proxy paths for pull orchestration to non-dynamic backends
      '/simplestore-pull': {
        target: 'http://localhost:8084',
        rewrite: (_path: string) => '/serviceorchestration/orchestration/pull',
      },
      '/flexiblestore-pull': {
        target: 'http://localhost:8085',
        rewrite: (_path: string) => '/serviceorchestration/orchestration/pull',
      },
    },
  },
  build: {
    outDir: 'dist',
  },
})
