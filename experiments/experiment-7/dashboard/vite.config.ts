import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Development proxy — routes /api/<prefix>/* to local services.
// For Docker deployment, nginx handles the same routing to container hostnames.
//
// Service host ports when running experiment-7 via docker compose:
//   consumerauth:      8082
//   rabbitmq mgmt:     15676  (mapped from container 15672)
//   authzforce:        8186
//   policy-sync:       9095
//   topic-auth-xacml:  9090
//   kafka-authz:       9091
//   cert-rest-authz:   9099   (HTTP health/status/auth-check port)
//   robot-fleet:       9106   (mapped from container 9003)
//   cert-consumer:     9096
//   analytics-consumer: 9004
//   ca:                8086

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
    port: 5178,
    proxy: {
      '/api/consumerauth': {
        target: 'http://localhost:8082',
        rewrite: path => path.replace(/^\/api\/consumerauth/, ''),
        changeOrigin: true,
      },
      '/api/rabbitmq': {
        target: 'http://localhost:15676',
        rewrite: path => path.replace(/^\/api\/rabbitmq/, ''),
        changeOrigin: true,
      },
      '/api/authzforce': {
        target: 'http://localhost:8186',
        rewrite: path => path.replace(/^\/api\/authzforce/, ''),
        changeOrigin: true,
      },
      '/api/policy-sync': {
        target: 'http://localhost:9095',
        rewrite: path => path.replace(/^\/api\/policy-sync/, ''),
        changeOrigin: true,
      },
      '/api/topic-auth-xacml': {
        target: 'http://localhost:9090',
        rewrite: path => path.replace(/^\/api\/topic-auth-xacml/, ''),
        changeOrigin: true,
      },
      '/api/kafka-authz': {
        target: 'http://localhost:9091',
        rewrite: path => path.replace(/^\/api\/kafka-authz/, ''),
        changeOrigin: true,
      },
      '/api/cert-rest-authz': {
        target: 'http://localhost:9099',
        rewrite: path => path.replace(/^\/api\/cert-rest-authz/, ''),
        changeOrigin: true,
      },
      '/api/robot-fleet': {
        target: 'http://localhost:9106',
        rewrite: path => path.replace(/^\/api\/robot-fleet/, ''),
        changeOrigin: true,
      },
      '/api/consumer-1': {
        target: 'http://localhost:9002',
        rewrite: path => path.replace(/^\/api\/consumer-1/, ''),
        changeOrigin: true,
      },
      '/api/consumer-2': {
        target: 'http://localhost:9002',
        rewrite: path => path.replace(/^\/api\/consumer-2/, ''),
        changeOrigin: true,
      },
      '/api/consumer-3': {
        target: 'http://localhost:9002',
        rewrite: path => path.replace(/^\/api\/consumer-3/, ''),
        changeOrigin: true,
      },
      '/api/analytics-consumer': {
        target: 'http://localhost:9004',
        rewrite: path => path.replace(/^\/api\/analytics-consumer/, ''),
        changeOrigin: true,
      },
      '/api/cert-consumer': {
        target: 'http://localhost:9096',
        rewrite: path => path.replace(/^\/api\/cert-consumer/, ''),
        changeOrigin: true,
      },
      '/api/ca': {
        target: 'http://localhost:8086',
        rewrite: path => path.replace(/^\/api\/ca/, ''),
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
