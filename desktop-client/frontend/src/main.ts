// main.ts mounts the Svelte application into the Wails frontend document.
import './style.css'
import App from './App.svelte'

const app = new App({
  target: document.getElementById('app')
})

export default app
