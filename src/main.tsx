import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './styles.css'

const metrika = window as Window & { ym?: ((...args: unknown[]) => void) & { a?: unknown[][]; l?: number } }
metrika.ym ||= (...args: unknown[]) => (metrika.ym!.a ||= []).push(args)
metrika.ym.l = Date.now()

const metrikaScript = document.createElement('script')
metrikaScript.async = true
metrikaScript.src = 'https://mc.yandex.ru/metrika/tag.js?id=113184214'
document.head.append(metrikaScript)

metrika.ym(113184214, 'init', {
  ssr: true,
  webvisor: true,
  clickmap: true,
  ecommerce: 'dataLayer',
  referrer: document.referrer,
  url: location.href,
  accurateTrackBounce: true,
  trackLinks: true,
})

createRoot(document.getElementById('root')!).render(
  <StrictMode><App /></StrictMode>,
)
