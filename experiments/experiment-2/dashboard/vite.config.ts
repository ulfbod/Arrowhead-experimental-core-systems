/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development, each /api/<prefix>/* path is rewritten and proxied to the
// corresponding local service.  In Docker, nginx handles the same mapping.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    proxy: {
      '/api/rabbitmq': {
        target: 'http://localhost:15672',
        rewrite: (path) => path.replace(/^\/api\/rabbitmq/, ''),
      },
      '/api/consumerauth': {
        target: 'http://localhost:8082',
        rewrite: (path) => path.replace(/^\/api\/consumerauth/, ''),
      },
      '/api/dynamicorch': {
        target: 'http://localhost:8083',
        rewrite: (path) => path.replace(/^\/api\/dynamicorch/, ''),
      },
      '/api/ca': {
        target: 'http://localhost:8086',
        rewrite: (path) => path.replace(/^\/api\/ca/, ''),
      },
      '/api/sr': {
        target: 'http://localhost:8080',
        rewrite: (path) => path.replace(/^\/api\/sr/, ''),
      },
      '/api/telemetry': {
        target: 'http://localhost:9001',
        rewrite: (path) => path.replace(/^\/api\/telemetry/, ''),
      },
      '/api/robot-fleet': {
        target: 'http://localhost:9003',
        rewrite: (path) => path.replace(/^\/api\/robot-fleet/, ''),
      },
      '/api/consumer-1': {
        target: 'http://localhost:9002',
        rewrite: (path) => path.replace(/^\/api\/consumer-1/, ''),
      },
      '/api/consumer-2': {
        target: 'http://localhost:9004',
        rewrite: (path) => path.replace(/^\/api\/consumer-2/, ''),
      },
      '/api/consumer-3': {
        target: 'http://localhost:9005',
        rewrite: (path) => path.replace(/^\/api\/consumer-3/, ''),
      },
      '/api/consumer': {
        target: 'http://localhost:9002',
        rewrite: (path) => path.replace(/^\/api\/consumer/, ''),
      },
    },
  },
  build: {
    outDir: 'dist',
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    coverage: { provider: 'v8', include: ['src/**'] },
  },
})
