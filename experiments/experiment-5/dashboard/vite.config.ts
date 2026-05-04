import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Development proxy — routes /api/<prefix>/* to local services.
// For Docker deployment, nginx handles the same routing to container hostnames.
//
// Service host ports when running experiment-5 via docker compose:
//   consumerauth:    8082
//   rabbitmq mgmt:   15675  (mapped from container 15672)
//   authzforce:      8180
//   policy-sync:     9095   (add ports: ["9095:9095"] to docker-compose for dev)
//   topic-auth-xacml: 9090  (add ports: ["9090:9090"] for dev)
//   kafka-authz:     9091
//   robot-fleet:     9105   (mapped from container 9003)

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5176,
    proxy: {
      '/api/consumerauth': {
        target: 'http://localhost:8082',
        rewrite: path => path.replace(/^\/api\/consumerauth/, ''),
        changeOrigin: true,
      },
      '/api/rabbitmq': {
        target: 'http://localhost:15675',
        rewrite: path => path.replace(/^\/api\/rabbitmq/, ''),
        changeOrigin: true,
      },
      '/api/authzforce': {
        target: 'http://localhost:8180',
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
      '/api/robot-fleet': {
        target: 'http://localhost:9105',
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
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    coverage: { provider: 'v8' },
  },
})
