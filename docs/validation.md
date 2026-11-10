# Проверка — 2026-11-10

## Локальные проверки

- Все пакеты CLI и browser bridge ранее прошли `go test -race ./...` и `make lint`; с тех пор Go-код, Go 1.25.0 в модуле и зависимости не менялись.
- `node --test tests/*.test.mjs` — 15 проверок прошли. Включены OAuth state/nonce, шифрование и срок сессии, CORS, проверка неизменяемого плана, защита назначения, повтор публикации и отказ при постороннем репозитории или другом HEAD.
- Проверка совместимости Worker сначала воспроизвела HTTP 500, затем прошла после привязки native fetch и замены неподдерживаемого `redirect: error` на `manual`. Ответы 3xx отклоняются; токены не следуют за перенаправлениями.
- `node scripts/build.mjs`, `node scripts/check-wasm.mjs`, `node scripts/validate-artifact.mjs` — прошли. Реальный WASM создал 365 дней и 415 точных timestamps; повтор и SHA-256 совпали.
- Git pack импортирован обычным Git, `git fsck --strict` прошёл, log подтвердил author/committer timestamps, noreply identity, manifest и число коммитов. Автотесты не отправляют запросы в настоящий GitHub.

## Опубликованный сайт

- Исходники: https://github.com/Manabreaker/activity-forge . Статика публикуется штатным branch-based Pages из `gh-pages`.
- https://manabreaker.github.io/activity-forge/ открыт в браузере. Генератор, изменение профиля/дат, фиксация плана и настоящий PNG-экспорт проверены.
- Мобильная сетка проверена без горизонтального переполнения при ширине 390 и 320 px. Временные viewport overrides сброшены.
- Hosted Actions не стартовал до выполнения шагов; он не учитывается как пройденная проверка. Workflow оставлен ручным.

## API и GitHub App

- API доступен: https://activity-forge-calendar.manabreaker.chatgpt.site . Первоначальная Cloudflare 403 больше не воспроизводится; `GET /api/config` возвращает HTTP 200 и `configured: true`. Запросы из реального Pages frontend проходят CORS.
- Зарегистрирован public GitHub App https://github.com/apps/activity-forge-by-manabreaker . Запрошены только Contents write, Metadata read и Repository creation write. Webhooks выключены. Client secret и SESSION_KEY настроены как runtime secrets Sites.
- Развёрнута Sites version 3: source `bec9d57afaf180c6da3b47838b6deea0d35553c7`, deployment `appgdep_6af36488c58881918ed6bb4ffc15ef05`, environment revision 2.
- Настоящий браузер прошёл вход через GitHub с S256 PKCE и вернулся в Pages. Адрес страницы после handoff очищен от fragment; профиль и реальный contributions calendar успешно загружены через App.
- Владелец подтвердил реальную установку в личный аккаунт в режиме Only select repositories. В сохранённых настройках GitHub выбран ровно один репозиторий; Contents write, Metadata read и Repository creation write подтверждены. Сайт распознал установку через API.
- На живом API подготовка отвергла существующий репозиторий до записи. Для нового имени сервер проверил зафиксированный план и выдал подтверждение с правильным назначением, количеством коммитов, периодом и ID плана. Диалог закрыт без публикации, исходные настройки восстановлены; HEAD существующей истории не изменился.
- Создание нового репозитория, фактический receive-pack через App, повтор живой публикации и отзыв grant пока не проверены на GitHub. Для отправки истории требуется отдельное явное подтверждение конкретного плана в интерфейсе. Существующая личная история не перепубликовывалась.

## Хранение данных

В коде приложения нет БД, дисковых Git-репозиториев, аналитики и записи токенов/планов в журналы. Проверка платформы выявила технические журналы Sites с метаданными и URL запроса, включая одноразовый код OAuth; это раскрыто в интерфейсе и [privacy.md](privacy.md). Абсолютное отсутствие любых записей у сторонних провайдеров не заявляется.
