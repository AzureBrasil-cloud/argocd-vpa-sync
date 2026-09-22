import { createBrowserRouter } from 'react-router-dom'
import { App } from './App'
import { RecommendationDetail } from './pages/RecommendationDetail'
import { RecommendationList } from './pages/RecommendationList'

export const router = createBrowserRouter([
  {
    path: '/',
    element: <App />,
    children: [
      { index: true, element: <RecommendationList /> },
      {
        path: 'recommendations/:namespace/:vpaName/:containerName',
        element: <RecommendationDetail />,
      },
    ],
  },
])
