import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // The Go server allows this origin by default (DASHBOARD_ORIGINS),
    // so fail instead of silently moving to another port.
    port: 5173,
    strictPort: true,
  },
})
