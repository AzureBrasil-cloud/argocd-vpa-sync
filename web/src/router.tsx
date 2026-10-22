import { createBrowserRouter } from 'react-router-dom'
import { App } from './App'
import { Bindings } from './pages/Bindings'
import { History } from './pages/History'
import { Login } from './pages/Login'
import { RecommendationDetail } from './pages/RecommendationDetail'
import { RecommendationList } from './pages/RecommendationList'
import { Settings } from './pages/Settings'

export const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  {
    path: '/',
    element: <App />,
    children: [
      { index: true, element: <RecommendationList /> },
      {
        path: 'recommendations/:namespace/:vpaName/:containerName',
        element: <RecommendationDetail />,
      },
      { path: 'history', element: <History /> },
      { path: 'bindings', element: <Bindings /> },
      { path: 'settings', element: <Settings /> },
    ],
  },
])
