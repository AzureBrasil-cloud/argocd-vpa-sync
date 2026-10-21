import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router-dom'
// Self-hosted faces, bundled so the dashboard needs no external font host
// (clusters reached over port-forward are often offline). A face only
// downloads when the active theme uses it.
import '@fontsource-variable/archivo/wdth.css'
import '@fontsource-variable/public-sans'
import '@fontsource-variable/jetbrains-mono'
import './index.css'
import { router } from './router'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
