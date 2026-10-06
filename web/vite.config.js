import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  // Absolute base path for the built assets. Defaults to "/", which is correct
  // for the dev server and for any host that serves the app at the domain root
  // (Netlify, Vercel, Cloudflare Pages). A host that serves the app from a
  // sub-path — GitHub Pages project sites, e.g.
  // https://<owner>.github.io/<repo>/ — must set this, or every hashed
  // /assets/* URL 404s. Pass it as VITE_BASE at build time (see
  // § "Deploying the frontend (static site)" in README.md).
  base: process.env.VITE_BASE || '/',
  plugins: [
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    tailwindcss(),
  ],
})