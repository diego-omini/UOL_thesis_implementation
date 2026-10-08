# Frontend

The Svelte and TypeScript frontend is built and embedded by Wails. Run these checks from `desktop-client`:

```powershell
npm test --prefix frontend
npm run check --prefix frontend
npm run build --prefix frontend
```

`src/App.svelte` owns the authenticated session boundary. Top-level authenticated screens are under `src/views`, reusable feature components are under `src/components`, and pure tested calculations and request-state rules are under `src/lib`.

Files below `wailsjs` and `dist` are generated. Regenerate Wails bindings with `wails generate module` and the embedded production assets with `npm run build --prefix frontend`; do not edit generated output directly.
