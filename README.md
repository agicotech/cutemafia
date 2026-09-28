# Cute mafia

Лендинг питомника шотландских кошек на React 19 + TypeScript + Vite.

## Локальный запуск

```bash
npm install
npm run dev
```

Во втором терминале:

```bash
python3 -m venv mock-api/.venv
mock-api/.venv/bin/pip install -r mock-api/requirements.txt
npm run dev:api
```

Frontend: `http://127.0.0.1:5173`. Отдельная страница конструктора: `http://127.0.0.1:5173/constructor/`. API и Swagger UI: `http://127.0.0.1:8000/docs`.

Пароль изменяющих API-запросов: `cutemafia-admin-2026`. Он захардкожен только в `mock-api/app.py`; сам конструктор открывается без входа и запрашивает пароль при первой попытке изменить данные.

## API

Контракт находится в `mock-api/openapi.yaml`. Mock-сервер хранит котят и заявки в JSON внутри `mock-api/data`, а загруженные изображения и видео — в `mock-api/uploads`.

Для другого API создайте `.env`:

```bash
VITE_API_URL=https://api.example.com
```

## Данные

Каталог, конструктор, медиа и контактная форма работают через HTTP API. Контактные данные в интерфейсе пока демонстрационные — замените константы `PHONE`, `TELEGRAM`, `TELEGRAM_CHANNEL`, `INSTAGRAM` и `WHATSAPP` в `src/App.tsx` перед публикацией.

## GitHub Pages

Workflow `.github/workflows/deploy.yml` собирает и публикует `dist` при каждом push в `main`. В настройках репозитория выберите **Settings → Pages → Source: GitHub Actions**.
