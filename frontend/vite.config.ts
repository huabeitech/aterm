import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import {resolve} from 'node:path'
export default defineConfig({
 plugins: [react(), tailwindcss()],
 resolve: {alias: {'@': resolve(__dirname, 'src/renderer/src')}},
 server: {host: '127.0.0.1', port: Number(process.env.WAILS_VITE_PORT) || 9245, strictPort: true},
 build: {chunkSizeWarningLimit: 2500}
})
