import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Development proxy — routes /api/<prefix>/* to local services.
// For Docker deployment, nginx handles the same routing to container hostnames.
//
// Service host ports when running experiment-3 via docker compose:
//   consumerauth:   8082
//   rabbitmq mgmt:  15673  (mapped from container 15672)
//   topic-auth-http: 9090  (add ports: ["9090:9090"] to docker-compose for dev)
//   robot-fleet:    9103   (mapped from container 9003)
//   consumer-1:     9002
//   consumer-2:     9004
//   consumer-3:     9005

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5175,
    proxy: {
      '/api/consumerauth': {
        target: 'http://localhost:8082',
        rewrite: path => path.replace(/^\/api\/consumerauth/, ''),
        changeOrigin: true,
      },
      '/api/rabbitmq': {
        target: 'http://localhost:15673',
        rewrite: path => path.replace(/^\/api\/rabbitmq/, ''),
        changeOrigin: true,
      },
      '/api/topic-auth-http': {
        target: 'http://localhost:9090',
        rewrite: path => path.replace(/^\/api\/topic-auth-http/, ''),
        changeOrigin: true,
      },
      '/api/robot-fleet': {
        target: 'http://localhost:9103',
        rewrite: path => path.replace(/^\/api\/robot-fleet/, ''),
        changeOrigin: true,
      },
      '/api/consumer-1': {
        target: 'http://localhost:9002',
        rewrite: path => path.replace(/^\/api\/consumer-1/, ''),
        changeOrigin: true,
      },
      '/api/consumer-2': {
        target: 'http://localhost:9004',
        rewrite: path => path.replace(/^\/api\/consumer-2/, ''),
        changeOrigin: true,
      },
      '/api/consumer-3': {
        target: 'http://localhost:9005',
        rewrite: path => path.replace(/^\/api\/consumer-3/, ''),
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    coverage: { provider: 'v8' },
  },
})
