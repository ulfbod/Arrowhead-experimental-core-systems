import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Development proxy — routes /api/<prefix>/* to local services.
// For Docker deployment, nginx handles the same routing to container hostnames.
//
// Service host ports when running experiment-6 via docker compose:
//   consumerauth:    8082
//   rabbitmq mgmt:   15676  (mapped from container 15672)
//   authzforce:      8186
//   policy-sync:     9095   (add ports: ["9095:9095"] to docker-compose for dev)
//   topic-auth-xacml: 9090  (add ports: ["9090:9090"] for dev)
//   kafka-authz:     9091
//   rest-authz:      9093
//   robot-fleet:     9106   (mapped from container 9003)
//   data-provider:   9094   (add ports: ["9094:9094"] to docker-compose for dev)
//   rest-consumer:   9097   (add ports: ["9097:9097"] to docker-compose for dev)

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5177,
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
      '/api/rest-authz': {
        target: 'http://localhost:9093',
        rewrite: path => path.replace(/^\/api\/rest-authz/, ''),
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
      '/api/data-provider': {
        target: 'http://localhost:9094',
        rewrite: path => path.replace(/^\/api\/data-provider/, ''),
        changeOrigin: true,
      },
      '/api/rest-consumer': {
        target: 'http://localhost:9097',
        rewrite: path => path.replace(/^\/api\/rest-consumer/, ''),
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
