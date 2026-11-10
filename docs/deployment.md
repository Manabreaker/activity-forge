# Развёртывание

## GitHub Pages

1. Создайте публичный репозиторий с этими исходниками.
2. Выполните локальные проверки из README, затем `node scripts/build.mjs` и `node scripts/check-wasm.mjs`.
3. Выполните `node scripts/publish-pages.mjs OWNER/REPO`. В Settings → Pages выберите Deploy from a branch → `gh-pages` → `/`.
4. При fork замените GitHub URL в интерфейсе/README; задайте адрес API в `web/config.js` и разрешите его в CSP `connect-src` файла `web/index.html`.

Секреты OAuth нельзя класть ни в Pages, ни в build-time переменные frontend, ни в Actions artifacts. Альтернатива при доступном Actions runner: выберите Source: GitHub Actions и вручную запустите workflow Publish GitHub Pages. Он проверяет исходники и публикует только `dist/pages`. В первоначальном окружении hosted runner был недоступен до выполнения шагов; branch-based Pages успешно собрался, поэтому используется он.

## Stateless API

Сервер — Worker ESM, Node compatibility, Web Crypto и HTTP. `worker/index.js` — исходный entrypoint; `node scripts/build.mjs` создаёт самостоятельный `dist/server/index.js`. Хранилища и service bindings не требуются. Для Cloudflare используется `wrangler.json`; для Sites — сохранённая `.openai/hosting.json` и стандартный source/version/deploy workflow.

Runtime configuration:

| Ключ | Тип | Значение |
|---|---|---|
| FRONTEND_URL | публичная настройка | полный Pages URL с путём и завершающим `/` |
| API_ORIGIN | публичная настройка | точный HTTPS origin API |
| GITHUB_APP_SLUG | публичная настройка | slug зарегистрированного приложения |
| GITHUB_CLIENT_ID | публичная настройка | client ID GitHub App, не App ID |
| GITHUB_CLIENT_SECRET | секрет | client secret приложения |
| SESSION_KEY | секрет | 32 случайных байта, закодированные base64 |

Для собственного Cloudflare аккаунта установите официальный Wrangler, авторизуйтесь и настройте секреты через stdin (`wrangler secret put`/`secret bulk`). Например, `openssl rand -base64 32 | npx wrangler secret put SESSION_KEY`. Не передавайте значения в аргументах команд и не сохраняйте `.env` в репозиторий. Замените API_ORIGIN в конфигурации на действительный Worker URL. Выполните `npx wrangler deploy` из корня проекта. Этот способ требует вашего аккаунта Cloudflare; GitHub Pages не размещает API.

## GitHub App

1. Откройте GitHub → Settings → Developer settings → GitHub Apps → New GitHub App.
2. Homepage: Pages URL. Callback: `API_ORIGIN/oauth/callback`. Setup URL: Pages URL. Webhooks выключены; webhook server не нужен.
3. Repository permissions: Contents **read/write**, Metadata **read**, Repository creation **write**. Account permissions не добавляйте. Не добавляйте Administration, Email, Workflows или доступ OAuth `repo`.
4. Для доступа друзьям разрешите установку другим аккаунтам (public GitHub App). Это не публикует ни их репозитории, ни историю.
5. Оставьте user access tokens с ограниченным временем жизни. Сервис дополнительно ограничивает свою сессию часом и не сохраняет refresh token.
6. Запишите client ID, slug и client secret только в runtime API; private key приложения этому варианту сервера не нужен.
7. Разверните API с этой конфигурацией. `GET /api/config` должен вернуть `configured: true` и правильный Pages URL. Frontend проверяет готовность самостоятельно.

Опциональный manifest без секретов — `github-app-manifest.json`; поле `redirect_url` используется только для создания App через manifest flow и не является OAuth callback. GitHub требует `hook_attributes.url` даже при `active: false`; доставка webhooks остаётся выключенной. Обычный ручной путь выше не требует manifest.

## Проверка перед открытием другим людям

- Проверьте реальный вход/отмену входа/возврат в Pages с nonce и очисткой fragment; сессия не должна появляться в URL после возврата.
- Проверьте установку в личный аккаунт с минимальным выбранным доступом. `repository_creation:write` появился 2026-10-13, но доступ нового репозитория и фактический ответ installations API нужно проверить на реальной установке. Не утверждайте изоляцию на уровне GitHub до этой проверки.
- Загрузите существующий календарь. Ошибка не должна превращаться в известные нулевые значения.
- Только после явного согласия владельца опубликуйте небольшой тестовый план в новый private repository. Проверьте log/count/dates/HEAD и повтор той же операции. Это отдельная живая проверка, не часть автотестов.
- Проверьте истечение сессии, повторный вход, восстановление того же имени репозитория, отзыв grant и ручное удаление установки.
- Проверьте и раскройте политику инфраструктурных журналов. Sites сохраняет технические метаданные и URL запроса; см. фактические границы в [privacy.md](privacy.md). Если требуется отсутствие таких записей, выберите хостинг с управляемым исключением чувствительных полей из журналов.

Ограничивайте абьюз/rate limits средствами провайдера, не собирая тела запросов и персональную аналитику. API принимает только настроенный Pages origin, а каждая операция публикации заново проверяет аккаунт и установку приложения. Совпадение Origin само по себе не заменяет аутентификацию.

## Официальные источники

- https://github.blog/changelog/2026-10-13-github-app-improvements-for-least-privilege-and-management/
- https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app
- https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/differences-between-github-apps-and-oauth-apps
