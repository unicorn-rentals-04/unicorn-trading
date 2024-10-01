import React from 'react';
import ReactDOM from 'react-dom/client';
import {createBrowserRouter, RouterProvider} from 'react-router-dom';
import {RecoilRoot} from 'recoil';
import App, {ErrorPage, Loading} from './App';
import {ArchiveReport, ArchivesBody} from './Archives';
import './index.css';
import {Login} from './Login';
import {OrderBody} from './Orders';
import {Pty} from './Pty';
import reportWebVitals from './reportWebVitals';

export function DevMode(): boolean {
  let func = 'Component'
  if (React['Component'].name === func) {
    return true
  }
  return false
}

export function BuildAppUrl(path: string): string {
  const devServer = "http://127.0.0.1:8080"
  const ip = process.env.REACT_APP_ECOMM_IP
  const prodServer = `http://${ip}:8081`
  return DevMode() ? `${devServer}${path}` : `${prodServer}${path}`
}

export function Root() {
  return <RecoilRoot>
    <App />
  </RecoilRoot>
}

const router = createBrowserRouter([
  {
    path: "/",
    element: <Root />,
    errorElement: <ErrorPage />,
    children: [{
      path: "/report",
      element: <React.Suspense fallback={<Loading />}>
        <OrderBody />
      </React.Suspense>
    },
    {
      path: "/archives",
      element: <React.Suspense fallback={<Loading />}>
        <ArchivesBody />
      </React.Suspense>
    },
    {
      path: "/archiveReport",
      element: <React.Suspense fallback={<Loading />}>
        <ArchiveReport />
      </React.Suspense>
    },
    {
      path: "/pty",
      element: <React.Suspense fallback={<Loading />}>
        <Pty />
      </React.Suspense>
    },
    {
      path: "/login",
      element: <React.Suspense fallback={<Loading />}>
        <Login />
      </React.Suspense>
    },
    ]
  },
]);


const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
