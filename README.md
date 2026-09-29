# Cute Mafia

Лендинг питомника и API каталога. Frontend написан на React, backend — на Go. API реализует контракт [`mock-api/openapi.yaml`](mock-api/openapi.yaml), хранит данные в SQLite или PostgreSQL, медиа — в Yandex Object Storage.

## Локальный запуск

```bash
npm install
cp backend/config.example.yaml backend/config.yaml
# Заполнить backend/config.yaml
npm run dev:api
```

Во втором терминале:

```bash
npm run dev
```

Frontend доступен на `http://127.0.0.1:5173`, страница питомника — на `/about/`, конструктор — на `/constructor/`, API — на `http://127.0.0.1:8000`. Рабочий `backend/config.yaml` содержит секреты и игнорируется Git.

## Backend

Поддерживаются `database.driver: sqlite` и `database.driver: postgres`. Одинаковые встроенные миграции применяются при старте. Смена драйвера не переносит существующие данные между БД.

Заявка сначала сохраняется в БД. Фоновый worker отправляет её в Telegram и при ошибке повторяет попытки по расписанию из YAML. Хост Telegram API можно переопределить через `telegram.api_host` — доменом или полным URL прокси. Второй worker раз в сутки удаляет из служебного префикса S3 медиа старше 24 часов, на которые не ссылается ни одна карточка.

В Yandex Object Storage необходимо:

1. Создать бакет и сервисный аккаунт со статическим ключом.
2. Разрешить анонимное чтение объектов, но не списка объектов и настроек.
3. Указать бакет, ключи и `public_base_url` в конфиге.

Фотографии перед отправкой уменьшаются до 2560 px и, если это сокращает размер, преобразуются браузером в WebP. Видео загружаются без транскодирования. Объекты получают годовой immutable cache; nginx сжимает текстовые ответы, но не пережимает уже сжатые изображения и видео.

## Сборка Linux/amd64 с macOS

```bash
scripts/build-backend.sh
```

Готовый статический бинарник появится в `dist/cute-mafia-api`.

## Первая установка на Ubuntu

До запуска направьте `api.cute-mafia.ru` на VPS, создайте production-конфиг и SSH-ключ для GitHub Actions. Затем передайте готовую сборку и bootstrap-файлы на сервер:

```bash
ssh-keygen -t ed25519 -f cute-mafia-deploy -C github-actions
ssh root@api.cute-mafia.ru 'mkdir -p /opt/cute-mafia-bootstrap'
scp -r dist backend scripts root@api.cute-mafia.ru:/opt/cute-mafia-bootstrap/
ssh root@api.cute-mafia.ru
sudo /opt/cute-mafia-bootstrap/scripts/install.sh \
  /opt/cute-mafia-bootstrap/dist/cute-mafia-api \
  /opt/cute-mafia-bootstrap/backend/config.yaml
```

Скрипт спросит email Let’s Encrypt и содержимое `cute-mafia-deploy.pub`, затем установит nginx, Certbot, systemd-сервис и отдельного пользователя `deploy`. Production-конфиг будет установлен как `/etc/cute-mafia/config.yaml` с правами `0600`.

Добавьте в GitHub Actions secrets:

- `VPS_HOST` — IP или SSH-host VPS;
- `VPS_SSH_KEY` — содержимое приватного `cute-mafia-deploy`;
- `VPS_KNOWN_HOSTS` — результат `ssh-keyscan -H api.cute-mafia.ru`.

Push в `main` тестирует API на SQLite и PostgreSQL, собирает Linux/amd64, обновляет systemd-сервис с health-check и rollback, затем публикует frontend на GitHub Pages с API `https://api.cute-mafia.ru`.

Полезные команды на VPS:

```bash
systemctl status cute-mafia-api
journalctl -u cute-mafia-api -f
nginx -t
```
