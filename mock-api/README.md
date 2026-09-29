# Cute mafia mock API

Локальный FastAPI-сервер для каталога, медиа и заявок. Реализует контракт `openapi.yaml` и дополнительно публикует Swagger UI на `/docs`.

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
.venv/bin/uvicorn app:app --reload --port 8000
```

Данные хранятся в `data/*.json`. Upload endpoint принимает изображения и видео до 100 МиБ.
