import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Development proxy — routes /api/<prefix>/* to local services.
// For Docker deployment, nginx handles the same routing to container hostnames.
//
// Service host ports when running experiment-8 via docker compose:
//   consumerauth:      8082  (plain HTTP — Docker-internal; only accessible inside Docker)
//   rabbitmq mgmt:     15677 (mapped from container 15672)
//   authzforce:        8196
//   policy-sync:       9105
//   kafka-authz:       9101
//   pki-rest-authz:    9109   (HTTP health/status/auth-check port)
//   robot-fleet:       9116   (mapped from container 9003)
//   pki-consumer:      9107
//   analytics-consumer: 9014  (mapped from container 9004)
//   profile-ca:        8087
//   serviceregistry:   8080  (plain HTTP — Docker-internal only)

export default defineConfig({
  plugins: [react()],
  resolve: {
    // src/main.tsx (and shared files) are symlinks to support/dashboard-shared/.
    // Without this, Rollup follows symlinks to the real path and resolves relative
    // imports from dashboard-shared/ — causing "Could not resolve './App'" at build time.
    // Docker builds are unaffected (Dockerfile removes symlinks before building).
    preserveSymlinks: true,
  },
  server: {
    port: 5179,
    proxy: {
      '/api/consumerauth': {
        target: 'http://localhost:8082',
        rewrite: path => path.replace(/^\/api\/consumerauth/, ''),
        changeOrigin: true,
      },
      '/api/rabbitmq': {
        target: 'http://localhost:15677',
        rewrite: path => path.replace(/^\/api\/rabbitmq/, ''),
        changeOrigin: true,
      },
      '/api/authzforce': {
        target: 'http://localhost:8196',
        rewrite: path => path.replace(/^\/api\/authzforce/, ''),
        changeOrigin: true,
      },
      '/api/policy-sync': {
        target: 'http://localhost:9105',
        rewrite: path => path.replace(/^\/api\/policy-sync/, ''),
        changeOrigin: true,
      },
      '/api/kafka-authz': {
        target: 'http://localhost:9101',
        rewrite: path => path.replace(/^\/api\/kafka-authz/, ''),
        changeOrigin: true,
      },
      '/api/pki-rest-authz': {
        target: 'http://localhost:9109',
        rewrite: path => path.replace(/^\/api\/pki-rest-authz/, ''),
        changeOrigin: true,
      },
      '/api/robot-fleet': {
        target: 'http://localhost:9116',
        rewrite: path => path.replace(/^\/api\/robot-fleet/, ''),
        changeOrigin: true,
      },
      '/api/pki-consumer': {
        target: 'http://localhost:9107',
        rewrite: path => path.replace(/^\/api\/pki-consumer/, ''),
        changeOrigin: true,
      },
      '/api/analytics-consumer': {
        target: 'http://localhost:9014',
        rewrite: path => path.replace(/^\/api\/analytics-consumer/, ''),
        changeOrigin: true,
      },
      '/api/profile-ca': {
        target: 'http://localhost:8087',
        rewrite: path => path.replace(/^\/api\/profile-ca/, ''),
        changeOrigin: true,
      },
      '/api/serviceregistry': {
        target: 'http://localhost:8080',
        rewrite: path => path.replace(/^\/api\/serviceregistry/, ''),
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    coverage: { provider: 'v8' },
  },
})
