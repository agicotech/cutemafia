import { useEffect, useMemo, useRef, useState } from 'react'
import { deleteKitten, getKittens, mediaUrl, optimizeImage, saveKitten, sendInquiry, uploadMedia } from './api'
import type { Inquiry, Kitten, MediaAsset, Price } from './types'

const PHONE = '+7 (903) 775-52-80'
const PHONE_LINK = '+79037755280'
const TELEGRAM = 'https://t.me/BSvetP'
const TELEGRAM_CHANNEL = 'https://t.me/tsezar689'
const INSTAGRAM = 'https://instagram.com/tsezar689'
const WHATSAPP = 'https://wa.me/79037755280'
const API_PASSWORD_KEY = 'cutemafia-api-password'

const emptyKitten = (): Kitten => ({
  id: '', name: '', birthDate: '', color: '', breedClass: 'Pet', generation: '',
  status: 'Свободен', description: '', mainPhoto: '', featuredVideo: null, media: [],
  prices: [{ label: 'Домашний любимец', value: '' }],
})

const emptyInquiry = (): Inquiry => ({ name: '', contact: '', message: '', kittenId: null })

const formatDate = (date: string) => new Intl.DateTimeFormat('ru-RU', {
  day: 'numeric', month: 'long', year: 'numeric',
}).format(new Date(`${date}T00:00:00`))

function PagePets() {
  const kittenRef = useRef<HTMLDivElement>(null)
  const yarnRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const kitten = kittenRef.current
    const yarn = yarnRef.current
    if (!kitten || !yarn || matchMedia('(prefers-reduced-motion: reduce)').matches) return

    const cat = { x: 24, y: innerHeight * .72, targetX: innerWidth * .55, targetY: innerHeight * .7, tilt: 0, restingUntil: 0, nextRest: performance.now() + 6000, kickAt: 0 }
    const ball = { x: innerWidth * .72, y: scrollY + innerHeight * .76, vx: 0, vy: 0, angle: 0 }
    const drag = { active: false, pointerId: -1, offsetX: 0, offsetY: 0, lastX: 0, lastY: 0, lastTime: 0 }
    let last = performance.now()
    let previousScrollY = scrollY
    let frame = 0

    const pickTarget = () => {
      cat.targetX = 18 + Math.random() * Math.max(20, innerWidth - 130)
      cat.targetY = innerHeight * (.58 + Math.random() * .25)
    }

    const grabBall = (event: PointerEvent) => {
      if (event.pointerType === 'mouse' && event.button !== 0) return
      const rect = yarn.getBoundingClientRect()
      drag.active = true
      drag.pointerId = event.pointerId
      drag.offsetX = event.clientX - rect.left
      drag.offsetY = event.clientY - rect.top
      drag.lastX = ball.x
      drag.lastY = ball.y
      drag.lastTime = event.timeStamp
      ball.vx = 0
      ball.vy = 0
      yarn.dataset.dragging = 'true'
      yarn.setPointerCapture(event.pointerId)
      event.preventDefault()
    }

    const dragBall = (event: PointerEvent) => {
      if (!drag.active || event.pointerId !== drag.pointerId) return
      const x = event.clientX - drag.offsetX
      const y = scrollY + event.clientY - drag.offsetY
      const elapsed = Math.max(8, event.timeStamp - drag.lastTime)
      ball.vx = Math.max(-35, Math.min(35, (x - drag.lastX) * 16.67 / elapsed))
      ball.vy = Math.max(-35, Math.min(35, (y - drag.lastY) * 16.67 / elapsed))
      ball.x = x
      ball.y = y
      drag.lastX = x
      drag.lastY = y
      drag.lastTime = event.timeStamp
      event.preventDefault()
    }

    const releaseBall = (event: PointerEvent) => {
      if (!drag.active || event.pointerId !== drag.pointerId) return
      const carry = Math.max(0, 1 - (event.timeStamp - drag.lastTime) / 120)
      ball.vx *= carry
      ball.vy *= carry
      drag.active = false
      yarn.removeAttribute('data-dragging')
      if (yarn.hasPointerCapture(event.pointerId)) yarn.releasePointerCapture(event.pointerId)
    }

    yarn.addEventListener('pointerdown', grabBall)
    yarn.addEventListener('pointermove', dragBall)
    yarn.addEventListener('pointerup', releaseBall)
    yarn.addEventListener('pointercancel', releaseBall)

    const animate = (now: number) => {
      const dt = Math.min((now - last) / 16.67, 2)
      last = now
      const ballScreenY = ball.y - scrollY
      const ballVisible = ballScreenY > 40 && ballScreenY < innerHeight - 20
      const resting = now < cat.restingUntil
      let desiredTilt = 0

      if (!resting && now > cat.nextRest) {
        cat.restingUntil = now + 2100 + Math.random() * 900
        cat.nextRest = cat.restingUntil + 5500 + Math.random() * 4500
      }

      if (!resting) {
        if (ballVisible && now > cat.kickAt) {
          cat.targetX = ball.x - 58
          cat.targetY = ballScreenY - 48
        } else if (Math.hypot(cat.targetX - cat.x, cat.targetY - cat.y) < 18) {
          pickTarget()
        }

        const dx = cat.targetX - cat.x
        const dy = cat.targetY - cat.y
        const distance = Math.hypot(dx, dy) || 1
        const speed = Math.min(2.7 * dt, distance)
        desiredTilt = Math.max(-16, Math.min(16, Math.atan2(dy, Math.abs(dx)) * 180 / Math.PI))
        cat.x += dx / distance * speed
        cat.y += dy / distance * speed
        kitten.dataset.frame = Math.floor(now / 135) % 2 ? 'run-a' : 'run-b'
        kitten.dataset.facing = dx < 0 ? 'left' : 'right'

        if (!drag.active && ballVisible && now > cat.kickAt && Math.hypot(ball.x - (cat.x + 52), ballScreenY - (cat.y + 44)) < 54) {
          ball.vx = (dx < 0 ? -1 : 1) * (4.5 + Math.random() * 2.5)
          ball.vy = (Math.random() - .5) * 3
          cat.kickAt = now + 1500
          pickTarget()
        }
      } else {
        kitten.dataset.frame = Math.floor(now / 650) % 2 ? 'sit-a' : 'sit-b'
      }

      const scrollDelta = scrollY - previousScrollY
      previousScrollY = scrollY
      if (!drag.active) {
        ball.x += ball.vx * dt
        ball.y += ball.vy * dt
        ball.vx *= Math.pow(.985, dt)
        ball.vy *= Math.pow(.955, dt)
        if (ball.x < 12 || ball.x > innerWidth - 38) {
          ball.x = Math.max(12, Math.min(innerWidth - 38, ball.x))
          ball.vx *= -.78
        }
        const viewportTop = scrollY + 12
        const viewportBottom = scrollY + innerHeight - 38
        const scrollSpeed = scrollDelta / dt
        const scrollImpulse = Math.min(22, 3.5 + Math.abs(scrollSpeed) * 1.35)
        if (ball.y < viewportTop) {
          ball.y = viewportTop + 1
          ball.vy = Math.max(Math.abs(ball.vy) * .78, scrollImpulse)
        } else if (ball.y > viewportBottom) {
          ball.y = viewportBottom - 1
          ball.vy = -Math.max(Math.abs(ball.vy) * .78, scrollImpulse)
        }
        ball.angle += ball.vx * dt * 2.4
      }

      cat.x = Math.max(8, Math.min(innerWidth - 100, cat.x))
      cat.y = Math.max(70, Math.min(innerHeight - 105, cat.y))
      cat.tilt += (desiredTilt - cat.tilt) * Math.min(1, .16 * dt)
      kitten.style.transform = `translate3d(${cat.x}px, ${cat.y}px, 0) scaleX(${kitten.dataset.facing === 'left' ? -1 : 1}) rotate(${cat.tilt}deg)`
      yarn.style.transform = `translate3d(${ball.x}px, ${ball.y}px, 0) rotate(${ball.angle}deg)`
      frame = requestAnimationFrame(animate)
    }

    frame = requestAnimationFrame(animate)
    return () => {
      cancelAnimationFrame(frame)
      yarn.removeEventListener('pointerdown', grabBall)
      yarn.removeEventListener('pointermove', dragBall)
      yarn.removeEventListener('pointerup', releaseBall)
      yarn.removeEventListener('pointercancel', releaseBall)
    }
  }, [])

  return <div className="page-pets" aria-hidden="true">
    <div ref={kittenRef} className="roaming-kitten" data-frame="run-a" data-facing="right"><span /></div>
    <div ref={yarnRef} className="rolling-yarn" />
  </div>
}

function KittenCard({ kitten, onClick }: { kitten: Kitten; onClick: () => void }) {
  const [showVideo, setShowVideo] = useState(false)
  const timer = useRef<number | null>(null)
  const startPreview = () => {
    if (!kitten.featuredVideo) return
    timer.current = window.setTimeout(() => setShowVideo(true), 1000)
  }
  const stopPreview = () => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
    setShowVideo(false)
  }
  useEffect(() => stopPreview, [])

  return <button className="kitten-card" onClick={onClick} onMouseEnter={startPreview} onMouseLeave={stopPreview} onFocus={startPreview} onBlur={stopPreview}>
    <img src={mediaUrl(kitten.mainPhoto)} alt="" loading="lazy" decoding="async" />
    {showVideo && kitten.featuredVideo && <video src={mediaUrl(kitten.featuredVideo)} muted loop playsInline autoPlay />}
    {kitten.featuredVideo && <span className="video-hint">Видео</span>}
    <span className="card-overlay"><span>{kitten.status}</span><strong>{kitten.name}</strong><small>{kitten.color}</small></span>
  </button>
}

function MediaView({ asset, className = '' }: { asset: MediaAsset; className?: string }) {
  return asset.mediaType === 'video'
    ? <video className={className} src={mediaUrl(asset.url)} controls playsInline autoPlay />
    : <img className={className} src={mediaUrl(asset.url)} alt={asset.name ?? ''} decoding="async" />
}

type SocialKind = 'telegram' | 'channel' | 'whatsapp' | 'instagram'

function SocialIcon({ kind }: { kind: SocialKind }) {
  if (kind === 'instagram') return <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="5" /><circle cx="12" cy="12" r="4" /><circle className="fill" cx="17.5" cy="6.5" r="1" /></svg>
  if (kind === 'whatsapp') return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11.7a8 8 0 0 1-11.9 7L4 20l1.3-4A8 8 0 1 1 20 11.7Z" /><path d="M9 8.5c.2-.5.5-.5.8-.5h.4c.2 0 .4.1.5.4l.8 1.8c.1.3 0 .5-.2.7l-.6.7c.8 1.6 2 2.7 3.7 3.3l.6-.8c.2-.3.5-.3.8-.2l1.8.9c.3.1.4.3.4.6 0 1.1-.7 2-1.7 2.3-1.1.3-2.7-.1-4.4-1.1-2.2-1.3-3.8-3.2-4.5-5.2-.4-1.2-.2-2.2.4-2.9Z" /></svg>
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 11 17-7-5 16-4-5-3 3 .6-4.4L17 7 9.8 12.7 3 11Z" />{kind === 'channel' && <path d="M5 5.5 2.5 3M4 8H1" />}</svg>
}

function AboutPage() {
  return <>
    <header className="topbar about-topbar">
      <a className="brand" href="../" aria-label="Cute mafia — на главную"><span className="brand-mark">CM</span><span>Cute mafia</span></a>
      <nav aria-label="Навигация"><a href="../#kittens">Котята</a><a href="../#contact">Контакты</a></nav>
    </header>
    <main className="about-page">
      <section className="about-hero">
        <div className="about-copy"><p className="eyebrow">О питомнике</p><h1>Растим характер, а не только породу</h1><p>Cute mafia — домашний питомник шотландских кошек. Мы внимательно относимся к здоровью линий, ранней социализации и тому, чтобы каждый котёнок рос рядом с человеком.</p><p>До переезда малыши знакомятся с обычной домашней жизнью, привыкают к рукам, звукам и общению. Будущим семьям мы честно рассказываем о темпераменте каждого котёнка и остаёмся на связи после переезда.</p></div>
        <figure className="about-image"><img src="../assets/kittens-duo.webp" alt="Шотландские котята питомника Cute mafia" /><figcaption>Москва · с заботой о каждом малыше</figcaption></figure>
      </section>
      <section className="about-values" aria-label="Принципы питомника">
        <article><span>01</span><h2>Здоровье</h2><p>Ответственно подбираем пары и следим за состоянием кошек и котят.</p></article>
        <article><span>02</span><h2>Характер</h2><p>Растим малышей дома, чтобы они были спокойными, контактными и доверяли людям.</p></article>
        <article><span>03</span><h2>Поддержка</h2><p>Помогаем подготовиться к переезду и отвечаем на вопросы новой семьи.</p></article>
      </section>
    </main>
    <footer><span className="brand-mark">CM</span><p>Cute mafia · питомник шотландских кошек</p><p>© {new Date().getFullYear()}</p></footer>
  </>
}

function App() {
  const [kittens, setKittens] = useState<Kitten[]>([])
  const [loadError, setLoadError] = useState('')
  const [selected, setSelected] = useState<Kitten | null>(null)
  const [activeMedia, setActiveMedia] = useState<MediaAsset | null>(null)
  const constructorRoute = location.pathname.replace(/\/+$/, '').endsWith('/constructor')
  const aboutRoute = location.pathname.replace(/\/+$/, '').endsWith('/about')
  const [draft, setDraft] = useState<Kitten>(emptyKitten)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [notice, setNotice] = useState('')
  const [uploading, setUploading] = useState(false)
  const [inquiry, setInquiry] = useState<Inquiry>(emptyInquiry)
  const [inquiryStatus, setInquiryStatus] = useState('')

  const refresh = async () => {
    try {
      setKittens(await getKittens())
      setLoadError('')
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : 'Не удалось загрузить каталог')
    }
  }

  useEffect(() => { void refresh() }, [])

  useEffect(() => {
    const openFromUrl = () => {
      const id = new URLSearchParams(location.search).get('kitten')
      const kitten = kittens.find((item) => item.id === id) ?? null
      setSelected(kitten)
      setActiveMedia(kitten ? { url: kitten.mainPhoto, mediaType: 'image', name: kitten.name } : null)
    }
    openFromUrl()
    addEventListener('popstate', openFromUrl)
    return () => removeEventListener('popstate', openFromUrl)
  }, [kittens])

  useEffect(() => {
    document.body.classList.toggle('locked', Boolean(selected || constructorRoute))
    return () => document.body.classList.remove('locked')
  }, [selected, constructorRoute])

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      if (selected) closeKitten()
    }
    addEventListener('keydown', closeOnEscape)
    return () => removeEventListener('keydown', closeOnEscape)
  }, [selected])

  useEffect(() => {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => entry.target.classList.toggle('is-visible', entry.isIntersecting))
    }, { threshold: 0.12 })
    document.querySelectorAll('.reveal').forEach((element) => observer.observe(element))
    return () => observer.disconnect()
  }, [kittens])

  useEffect(() => {
    const modelContext = (document as Document & { modelContext?: { registerTool: (tool: unknown, options?: unknown) => unknown } }).modelContext
    if (!modelContext?.registerTool) return
    const lifecycle = new AbortController()
    const register = (tool: unknown) => Promise.resolve(modelContext.registerTool(tool, { signal: lifecycle.signal })).catch(() => undefined)
    void register({
      name: 'list_kittens', title: 'Список котят', description: 'Возвращает список котят питомника и их поколения.',
      inputSchema: { type: 'object', properties: {}, additionalProperties: false },
      annotations: { readOnlyHint: true, untrustedContentHint: false },
      execute: () => kittens.map(({ id, name, generation, status }) => ({ id, name, generation, status })),
    })
    void register({
      name: 'open_kitten', title: 'Открыть карточку котёнка', description: 'Открывает карточку котёнка по идентификатору.',
      inputSchema: { type: 'object', properties: { id: { type: 'string' } }, required: ['id'], additionalProperties: false },
      annotations: { readOnlyHint: false, untrustedContentHint: false },
      execute: (input: { id: string }) => {
        const kitten = kittens.find((item) => item.id === input.id)
        if (!kitten) throw new Error('Котёнок не найден')
        openKitten(kitten)
        return { opened: kitten.id, name: kitten.name }
      },
    })
    return () => lifecycle.abort()
  }, [kittens])

  const generations = useMemo(() => {
    const groups = new Map<string, Kitten[]>()
    kittens.forEach((kitten) => groups.set(kitten.generation, [...(groups.get(kitten.generation) ?? []), kitten]))
    return [...groups.entries()]
  }, [kittens])

  const gallery = selected ? [
    { url: selected.mainPhoto, mediaType: 'image' as const, name: selected.name },
    ...(selected.featuredVideo ? [{ url: selected.featuredVideo, mediaType: 'video' as const, name: `Видео ${selected.name}` }] : []),
    ...selected.media,
  ] : []

  const openKitten = (kitten: Kitten) => {
    const url = new URL(location.href)
    url.searchParams.set('kitten', kitten.id)
    history.pushState({}, '', url)
    setSelected(kitten)
    setActiveMedia({ url: kitten.mainPhoto, mediaType: 'image', name: kitten.name })
  }

  const closeKitten = () => {
    const url = new URL(location.href)
    url.searchParams.delete('kitten')
    history.pushState({}, '', url)
    setSelected(null)
  }

  const startInquiry = (kitten?: Kitten) => {
    closeKitten()
    setInquiry((current) => ({ ...current, kittenId: kitten?.id ?? null, message: kitten ? `Хочу познакомиться с котёнком ${kitten.name}.` : current.message }))
    requestAnimationFrame(() => document.querySelector('#contact')?.scrollIntoView({ behavior: 'smooth' }))
  }

  const editKitten = (kitten?: Kitten) => {
    setEditingId(kitten?.id ?? null)
    setDraft(kitten ? structuredClone(kitten) : emptyKitten())
  }

  const apiPassword = () => {
    const password = sessionStorage.getItem(API_PASSWORD_KEY) ?? prompt('Пароль администратора')?.trim()
    if (!password) throw new Error('Для изменения данных нужен пароль администратора')
    sessionStorage.setItem(API_PASSWORD_KEY, password)
    return password
  }

  const adminError = (error: unknown, fallback: string) => {
    if (error instanceof Error && error.message === 'Неверный пароль') sessionStorage.removeItem(API_PASSWORD_KEY)
    return error instanceof Error ? error.message : fallback
  }

  const handleUpload = async (files: FileList | null, target: 'main' | 'featured' | 'gallery') => {
    if (!files?.length) return
    setUploading(true)
    setNotice('Загружаем медиа…')
    try {
      const password = apiPassword()
      const assets: MediaAsset[] = []
      for (const file of files) assets.push(await uploadMedia(await optimizeImage(file), password))
      setDraft((current) => target === 'main'
        ? { ...current, mainPhoto: assets[0].url }
        : target === 'featured'
          ? { ...current, featuredVideo: assets[0].url }
          : { ...current, media: [...current.media, ...assets] })
      setNotice('Медиа загружено')
    } catch (error) {
      setNotice(adminError(error, 'Не удалось загрузить файл'))
    } finally {
      setUploading(false)
    }
  }

  const submitKitten = async (event: React.FormEvent) => {
    event.preventDefault()
    try {
      await saveKitten({ ...draft, id: editingId ?? '' }, apiPassword())
      await refresh()
      editKitten()
      setNotice('Изменения сохранены')
    } catch (error) {
      setNotice(adminError(error, 'Не удалось сохранить'))
    }
  }

  const removeKitten = async (id: string) => {
    if (!confirm('Удалить котёнка из каталога?')) return
    try {
      await deleteKitten(id, apiPassword())
      await refresh()
      if (editingId === id) editKitten()
    } catch (error) {
      setNotice(adminError(error, 'Не удалось удалить'))
    }
  }

  const submitInquiry = async (event: React.FormEvent) => {
    event.preventDefault()
    setInquiryStatus('Отправляем…')
    try {
      await sendInquiry(inquiry)
      setInquiry(emptyInquiry())
      setInquiryStatus('Спасибо! Заявка отправлена, мы скоро свяжемся с вами.')
    } catch (error) {
      setInquiryStatus(error instanceof Error ? error.message : 'Не удалось отправить заявку')
    }
  }

  return <>
    {aboutRoute && <AboutPage />}
    {!constructorRoute && !aboutRoute && <>
    <header className="topbar">
      <a className="brand" href="#top" aria-label="Cute mafia — на главную"><span className="brand-mark">CM</span><span>Cute mafia</span></a>
      <nav aria-label="Основная навигация"><a href="./about/">О питомнике</a><a href="#kittens">Котята</a><a href="#contact">Контакты</a></nav>
    </header>

    <main id="top">
      <section className="story reveal">
        <div className="story-copy"><p className="eyebrow">Питомник шотландских кошек · Москва</p><h1>Семья начинается с мурчания</h1><p className="lead">Cute mafia началась с одной маленькой шотландской кошки и большой любви к её характеру. Мы бережно растим малышей, следим за здоровьем линий и знаем привычки каждого котёнка ещё до вашей первой встречи.</p><a className="round-link" href="#kittens">Познакомиться с котятами <span>↓</span></a></div>
        <figure className="story-image"><img src={`${import.meta.env.BASE_URL}assets/kittens-duo.webp`} alt="Два шотландских котёнка Cute mafia" /><figcaption><span>04</span> поколения нашей котомафии</figcaption></figure>
      </section>

      <section className="catalog" id="kittens">
        <div className="section-heading reveal"><p className="eyebrow">Наши выпускники и малыши</p><h2>Поколения</h2></div>
        {loadError && <div className="api-error"><p>API недоступно: {loadError}</p><button className="primary-button" onClick={() => void refresh()}>Повторить</button></div>}
        {generations.map(([generation, items]) => <section className="generation reveal" key={generation}><div className="generation-title"><span>ID</span><div><h3>Поколение {generation}</h3><p>{items.length} {items.length === 1 ? 'котёнок' : 'котёнка'}</p></div></div><div className="kitten-grid">{items.map((kitten) => <KittenCard kitten={kitten} key={kitten.id} onClick={() => openKitten(kitten)} />)}</div></section>)}
      </section>

      <section className="contact reveal" id="contact">
        <div className="contact-copy"><p className="eyebrow">Давайте знакомиться</p><h2>Расскажите, кого вы ищете</h2><p>Ответим на вопросы о характере, документах и переезде котёнка в новый дом.</p><div className="direct-contacts"><a href={`tel:${PHONE_LINK}`}><small>Позвонить</small><strong>{PHONE}</strong></a></div><div className="social-links" aria-label="Социальные сети"><a href={TELEGRAM} target="_blank" rel="noreferrer" aria-label="Telegram" title="Telegram"><SocialIcon kind="telegram" /></a><a href={WHATSAPP} target="_blank" rel="noreferrer" aria-label="WhatsApp" title="WhatsApp"><SocialIcon kind="whatsapp" /></a><a href={INSTAGRAM} target="_blank" rel="noreferrer" aria-label="Instagram" title="Instagram"><SocialIcon kind="instagram" /></a><a href={TELEGRAM_CHANNEL} target="_blank" rel="noreferrer" aria-label="Telegram-канал" title="Telegram-канал"><SocialIcon kind="channel" /></a></div></div>
        <form className="contact-form" onSubmit={submitInquiry}>
          <label>Ваше имя<input required minLength={2} value={inquiry.name} onChange={(e) => setInquiry({ ...inquiry, name: e.target.value })} /></label>
          <label>Телефон, Telegram или WhatsApp<input required minLength={3} value={inquiry.contact} onChange={(e) => setInquiry({ ...inquiry, contact: e.target.value })} /></label>
          <label>Сообщение<textarea required minLength={3} rows={5} value={inquiry.message} onChange={(e) => setInquiry({ ...inquiry, message: e.target.value })} /></label>
          {inquiry.kittenId && <p className="selected-kitten">Выбран котёнок: <strong>{kittens.find((item) => item.id === inquiry.kittenId)?.name}</strong> <button type="button" onClick={() => setInquiry({ ...inquiry, kittenId: null })}>×</button></p>}
          <button className="primary-button" type="submit">Отправить заявку</button>{inquiryStatus && <p className="form-status" role="status">{inquiryStatus}</p>}
        </form>
      </section>
    </main>

    <footer><span className="brand-mark">CM</span><p>Cute mafia · питомник шотландских кошек</p><p>© {new Date().getFullYear()}</p></footer>

    {!constructorRoute && <PagePets />}

    {selected && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeKitten() }}><article className="kitten-modal" role="dialog" aria-modal="true" aria-labelledby="kitten-name">
      <button className="close" onClick={closeKitten} aria-label="Закрыть">×</button><div className="modal-head"><p className="eyebrow">Поколение {selected.generation}</p><h2 id="kitten-name">{selected.name}</h2><span className="status">{selected.status}</span></div>
      {activeMedia && <div className="gallery"><MediaView className="main-photo" asset={activeMedia} /><div className="thumbs">{gallery.map((asset, index) => <button className={activeMedia.url === asset.url ? 'active' : ''} key={`${asset.url}-${index}`} onClick={() => setActiveMedia(asset)} aria-label={`${asset.mediaType === 'video' ? 'Видео' : 'Фотография'} ${index + 1}`}>{asset.mediaType === 'video' ? <video src={mediaUrl(asset.url)} muted preload="metadata" /> : <img src={mediaUrl(asset.url)} alt="" loading="lazy" decoding="async" />}</button>)}</div></div>}
      <dl className="facts"><div><dt>Дата рождения</dt><dd>{formatDate(selected.birthDate)}</dd></div><div><dt>Окрас</dt><dd>{selected.color}</dd></div><div><dt>Класс</dt><dd>{selected.breedClass}</dd></div></dl><div className="prices">{selected.prices.map((price) => <div key={`${price.label}-${price.value}`}><span>{price.label}</span><strong>{price.value}</strong></div>)}</div><div className="description"><p className="eyebrow">О котёнке</p><p>{selected.description}</p></div><div className="modal-actions"><button className="primary-button kitten-contact" onClick={() => startInquiry(selected)}>Оставить заявку</button><a className="text-button" href={TELEGRAM} target="_blank" rel="noreferrer">Написать в Telegram</a></div>
    </article></div>}
    </>}

    {constructorRoute && <div className="builder-shell" aria-labelledby="builder-title"><header><div><p className="eyebrow">Данные API</p><h2 id="builder-title">Конструктор котят</h2></div><div className="admin-actions"><a className="text-button" href="../">На сайт</a></div></header><div className="builder-layout">
      <aside><button className="primary-button" onClick={() => editKitten()}>+ Добавить котёнка</button><div className="builder-list">{kittens.map((kitten) => <div className={editingId === kitten.id ? 'active' : ''} key={kitten.id}><button onClick={() => editKitten(kitten)}><img src={mediaUrl(kitten.mainPhoto)} alt="" loading="lazy" decoding="async" /><span><strong>{kitten.name}</strong><small>Поколение {kitten.generation}</small></span></button><button className="delete" onClick={() => void removeKitten(kitten.id)} aria-label={`Удалить ${kitten.name}`}>×</button></div>)}</div></aside>
      <form onSubmit={submitKitten}><div className="form-title"><h3>{editingId ? `Редактирование: ${draft.name}` : 'Новый котёнок'}</h3>{notice && <span className="notice">{notice}</span>}</div><div className="form-grid">
        <label>Имя<input required value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} /></label><label>Дата рождения<input required type="date" value={draft.birthDate} onChange={(e) => setDraft({ ...draft, birthDate: e.target.value })} /></label><label>Окрас<input required value={draft.color} onChange={(e) => setDraft({ ...draft, color: e.target.value })} /></label><label>Класс<select value={draft.breedClass} onChange={(e) => setDraft({ ...draft, breedClass: e.target.value })}><option>Pet</option><option>Breed</option><option>Show</option></select></label><label>ID поколения<input required maxLength={80} placeholder="Например, spring-2026" value={draft.generation} onChange={(e) => setDraft({ ...draft, generation: e.target.value })} /></label><label>Статус<input required value={draft.status} onChange={(e) => setDraft({ ...draft, status: e.target.value })} /></label>
      </div><label>Описание<textarea required rows={5} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} /></label>
      <fieldset><legend>Фото и видео</legend><div className="photo-fields"><label>Главное фото<input disabled={uploading} required={!draft.mainPhoto} type="file" accept="image/*" onChange={(e) => void handleUpload(e.target.files, 'main')} /></label><label>Главное hover-видео<input disabled={uploading} type="file" accept="video/*" onChange={(e) => void handleUpload(e.target.files, 'featured')} /></label><label>Галерея<input disabled={uploading} type="file" multiple accept="image/*,video/*" onChange={(e) => void handleUpload(e.target.files, 'gallery')} /></label></div>{(draft.mainPhoto || draft.featuredVideo || draft.media.length > 0) && <div className="photo-preview">{draft.mainPhoto && <img src={mediaUrl(draft.mainPhoto)} alt="Главная фотография" />}{draft.featuredVideo && <button type="button" onClick={() => setDraft({ ...draft, featuredVideo: null })} title="Удалить главное видео"><video src={mediaUrl(draft.featuredVideo)} muted /><span>×</span><small>Hover</small></button>}{draft.media.map((asset, index) => <button type="button" key={`${asset.url}-${index}`} onClick={() => setDraft({ ...draft, media: draft.media.filter((_, itemIndex) => itemIndex !== index) })} title="Удалить медиа">{asset.mediaType === 'video' ? <video src={mediaUrl(asset.url)} muted /> : <img src={mediaUrl(asset.url)} alt="" />}<span>×</span></button>)}</div>}</fieldset>
      <fieldset><legend>Цены</legend>{draft.prices.map((price, index) => <div className="price-row" key={index}><input aria-label="Название цены" placeholder="Например, домашний любимец" value={price.label} onChange={(e) => setDraft({ ...draft, prices: draft.prices.map((item, itemIndex) => itemIndex === index ? { ...item, label: e.target.value } : item) })} /><input aria-label="Цена" placeholder="95 000 ₽" value={price.value} onChange={(e) => setDraft({ ...draft, prices: draft.prices.map((item, itemIndex) => itemIndex === index ? { ...item, value: e.target.value } : item) })} /><button type="button" onClick={() => setDraft({ ...draft, prices: draft.prices.filter((_, itemIndex) => itemIndex !== index) })} aria-label="Удалить цену">×</button></div>)}<button className="text-button add-price" type="button" onClick={() => setDraft({ ...draft, prices: [...draft.prices, { label: '', value: '' } as Price] })}>+ Добавить цену</button></fieldset><button className="primary-button save" disabled={uploading} type="submit">Сохранить котёнка</button>
      </form>
    </div></div>}
  </>
}

export default App
