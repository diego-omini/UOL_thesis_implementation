// vite.config.ts configures Vite to compile the Svelte frontend for Wails.
import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()]
})
