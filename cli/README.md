# Activity Forge

Для пересылки другу доступен отдельный запуск с русским меню:

```sh
go build -trimpath -o activityforge-friend ./cmd/friend
./activityforge-friend
```

Он сам подставляет имя/noreply текущего GitHub-аккаунта, предлагает три спокойных профиля, сохраняет данные отдельно для каждого аккаунта и проводит через browser preview к подтверждению публикации. Go нужен только при самостоятельной сборке; готовые бинарники поставляются отдельно для Windows/macOS/Linux, x64/ARM64. Windows-сборка использует системную блокировку файла и открывает браузер через `rundll32.exe`. Для GitHub-публикации через меню требуются системные Git и GitHub CLI.

Локальный Go CLI для **декоративной** истории GitHub Contributions. Сначала настройте календарь в терминале или браузере, сохраните план с точными timestamps, создайте отдельный Git-репозиторий, проверьте настоящий `git log` и только затем подтвердите публикацию.

Go 1.25+, Git; macOS Apple Silicon и Linux. Внешних Go/JS-зависимостей нет; база IANA timezone встроена в бинарник для систем без установленного tzdata. `gh` нужен только для удобной авторизации GitHub; доступ по токену также поддерживается. Локальные preview и экспорты работают без GitHub и интернета.

## Быстрый старт

```sh
go build
./activityforge

# Либо браузерный мастер с пересчётом через Go backend:
./activityforge preview --open --days 365 --preset active-developer --seed 42
```

`preview --open` запускает сервер на случайном порту `127.0.0.1`, открывает браузер и работает до Ctrl+C. Для headless-среды: `preview --serve --port 7342`. Порт наружу не публикуется. `init` сохраняет исходную конфигурацию, `configure` запускает терминальный мастер без создания коммитов.

По умолчанию состояние находится в пользовательском config-каталоге `activityforge` (`~/Library/Application Support/activityforge` в macOS, `$XDG_CONFIG_HOME/activityforge` либо `~/.config/activityforge` в Linux). Все примеры можно изолировать:

```sh
export ACTIVITYFORGE_STATE_DIR="${TMPDIR:-/tmp}/activityforge-demo"
./activityforge init
./activityforge preview --open --offline
```

Состояние: `config.json`, `github-cache.json`, `plans/<sha256>.json`, `manifests/<sha256>.json`, `last-plan`. История создаётся отдельно в `STATE/history`, либо по `--repo PATH`. В рабочем дереве генерируемого репозитория нет конфигурации или данных; коммиты используют пустое дерево. Служебная отметка владения лежит внутри `.git/activityforge.json`.

## Настройка и preview

```sh
./activityforge preview \
  --start 2025-11-11 --end 2026-11-10 \
  --preset active-developer --intensity 7 --randomness 60 \
  --weekends light --seed 42 --open

./activityforge preview --days 90 --preset chaotic \
  --weekends custom --saturday-probability .6 --saturday-multiplier .4 --saturday-max 5 \
  --sunday-probability .15 --sunday-multiplier .2 --sunday-max 2
```

Пресеты `balanced`, `active-developer`, `hardcore`, `chaotic` задают начальные значения, после чего отдельные flags их переопределяют. `custom` оставляет сохранённые значения. Probability — число **0–1**, randomness/inactive — проценты **0–100**. Для индивидуальных настроек субботы и воскресенья выберите `--weekends custom`.

`--mean 0` выводит нагрузку из intensity, положительный `--mean` задаёт среднюю нагрузку **активного рабочего дня до множителей и ограничений**. `--inactive` — базовая вероятность неактивного буднего дня; отдых, отпуска и streaks дополнительно меняют её. `--max` ограничивает целевой ежедневный объём и количество новых коммитов; уже существующие contributions не урезаются. `--min` ограничивает целевой активный день, а разница после вычитания существующих contributions может быть меньше.

Эффекты отключаются независимо:

```sh
./activityforge preview --bursts=false --rest=false --streaks=false \
  --monthly=false --vacations 0 --trend stable --randomness 0
```

Vacation count задаёт число выбираемых интервалов; в коротких периодах интервалы могут пересекаться и обрезаются границами периода. Длительности задаются `--vacation-min` / `--vacation-max`. Короткие rest periods длятся 1–4 дня, work bursts — несколько дней или недель. Месячные множители плавно интерполируются, соседние дневные и недельные множители коррелированы. Времена распределены по нерегулярным рабочим сессиям, вечерним и ночным сериям.

В браузере доступны даты, пресеты, intensity, randomness, выходные (включая разные параметры субботы/воскресенья), все эффекты, лимиты, автор, часовой пояс, времена и seed. **Regenerate** меняет seed. **Reset** восстанавливает параметры, с которыми открыта страница. **Save this plan** сохраняет и замораживает выбранный результат; **New draft** позволяет экспериментировать дальше, оставляя сохранённый файл неизменным.

В терминальном мастере после preview доступны отдельные изменения intensity/randomness/weekends/dates, новый seed, экспорты, открытие браузера, сохранение плана и локальная генерация. Терминальный календарь имеет 7 строк, подписи месяцев, пять уровней, автоматический перенос по ширине и режим `--no-color` / `NO_COLOR`.

## Существующая активность GitHub

```sh
gh auth login
./activityforge preview --days 365 --fetch --open
```

Получение использует GraphQL `contributionsCollection.contributionCalendar`: `contributionCount`, `contributionLevel`, `color`, `date`, `weekday`, `totalContributions`, `colors`. Периоды больше года разбиваются на окна до 300 дней. Кэш можно использовать полностью офлайн.

- **CURRENT**: только сохранённые реальные данные GitHub с цветами и уровнями API.
- **PREVIEW**: `existing + new`.
- **DIFF**: только добавляемые коммиты.

Для дня `new = max(0, target − existing)`. Если реальная активность выше цели, новые коммиты не нужны. Без загруженного календаря интерфейс явно показывает неизвестную исходную активность; нули не выдаются за проверенные данные.

Кнопка **Load / refresh GitHub baseline** загружает/обновляет календарь для выбранных дат. Автоматический кэш вне выбранного периода игнорируется с сообщением; частично покрытый календарь не считается полной исходной активностью.

`--no-existing` полностью отключает учёт реальных contributions и кэша. `--existing snapshot.json` использует явно выбранный JSON Snapshot; `--offline` запрещает сетевые запросы, но допускает уже сохранённый кэш.

Без `gh`: создайте пустой standalone-репозиторий вручную на GitHub, задайте `main` default branch и передайте `GH_TOKEN` либо `GITHUB_TOKEN` через окружение (не сохраняйте в JSON). Тогда REST/GraphQL и обычный HTTPS push выполняются без GitHub CLI. Для приватного календаря, email и публикации токену нужны соответствующие разрешения к профилю/email и целевому репозиторию. Ошибка доступа не заменяется успехом; для email без необходимого scope используйте подтверждённый account noreply.

## Сохранить и применить именно увиденный план

```sh
./activityforge plan --days 365 --preset active-developer --seed 42 \
  --author-name 'Your Name' \
  --author-email '123456+your-login@users.noreply.github.com' \
  --output activity-plan.json

./activityforge preview --plan activity-plan.json --open
./activityforge generate --plan activity-plan.json
./activityforge inspect --plan activity-plan.json --open
./activityforge status --plan activity-plan.json
```

Подставьте **свой** GitHub noreply либо verified email. Локальный preview допускает пустую identity; генерация требует явных name/email. Перед публикацией identity проверяется через GitHub, в том числе принадлежность noreply текущему аккаунту.

План содержит version, настройки, seed, исходный календарь, ежедневные цели и новые коммиты, точные author/committer timestamps и SHA-256 всего содержимого. `generate` ничего не пересчитывает. Изменение файла обнаруживается при загрузке, существующий план нельзя перезаписать другим содержимым, flags генерации вместе с `--plan` отклоняются.

Повторное применение того же плана сверяется с настоящей историей и не добавляет дубликаты. После Ctrl+C/сбоя команда продолжает с первого отсутствующего индекса, включая случай, когда Git ref уже продвинулся, а manifest ещё не записан. Блокировка освобождается ядром при завершении процесса. Новые планы не должны пересекаться с уже созданными датами и должны следовать за ними; для другого визуального варианта используйте **новую пустую директорию**. Утилита не переписывает созданную историю. Удалять или сбрасывать существующую историю автоматически она не будет.

`inspect` читает настоящие author dates и committer dates, имена/email, индексы, родительские связи и деревья. Показывает только коммиты выбранного плана. Посторонний коммит, другой файл в дереве, пропуск, неверная identity/date, изменённая ветка или незакоммиченные изменения блокируют публикацию. В случае расхождений browser preview показывает фактические коммиты и список проблем, а текстовая команда завершается с ошибкой.

## Подтверждаемая публикация

```sh
./activityforge publish --plan activity-plan.json --remote your-login/activity-calendar

# Если удалённого репозитория ещё нет:
./activityforge publish --plan activity-plan.json \
  --remote your-login/activity-calendar --create-private

# Headless: итоговый график в терминале и точная текстовая фраза подтверждения
./activityforge publish --plan activity-plan.json \
  --remote your-login/activity-calendar --terminal
```

Перед подтверждением проверяются настоящий Git log, account/email, standalone-репозиторий (fork не подходит), `main` как default branch, ancestry удалённой ветки и ожидаемый HEAD. Текущий календарь по возможности обновляется. Финальный browser preview неизменяем: **Approve & Push** требует явного подтверждения, повторно проверяет локальный/удалённый HEAD под блокировкой и выполняет обычный fast-forward push. Создание приватного репозитория также выполняется только после этого подтверждения. Страница сама ничего не публикует. Сетевые ошибки требуют новой подготовки и нового одобрения.

**Back to settings** отменяет старое одобрение и открывает новый draft в той же странице. Созданные Git-коммиты при этом не меняются. Изменённый план сохраните отдельно и примените в новой пустой директории через `generate --repo NEW_PATH`; это явная операция, существующая история не переписывается. В финальном preview также доступны экспорт и **Cancel publication**.

Уже отправленные коммиты не считаются новыми и повторно не отправляются. Если локальная или удалённая история изменилась после preview, нужна новая проверка и подтверждение. Репозитории с чужой историей не объединяются; force push и переписывание истории отсутствуют. В одном репозитории публикуйте предыдущие планы прежде, чем добавлять следующий.

После push на GitHub откройте профиль → **Contribution settings → Private contributions** для отображения приватной активности. Коммиты должны попасть в default branch standalone-репозитория и принадлежать verified/noreply email аккаунта. GitHub может индексировать изменения до 24 часов. Calendar contributions включают не только commits (также PR/issues/reviews), поэтому их типы не эквивалентны.

## Экспорт

```sh
./activityforge preview --plan activity-plan.json --output preview.html
./activityforge preview --plan activity-plan.json --output preview.png
./activityforge preview --plan activity-plan.json --format svg --output preview.svg
./activityforge preview --plan activity-plan.json --format csv --output days.csv
./activityforge preview --plan activity-plan.json --format json > exact-plan.json
./activityforge inspect --plan activity-plan.json --format json --output inspection.json
```

HTML автономен: внутри находятся все стили, JavaScript и данные. Переключение CURRENT/PREVIEW/DIFF, темы, tooltips, сведения о timestamps и экспорт работают без сервера. Изменять параметры/seed в автономном снимке нельзя: для пересчёта нужен Go backend. PNG/SVG/CSV/JSON строятся по ежедневным значениям реального плана, без заглушек. Native PNG использует небольшой встроенный bitmap font; браузер также умеет экспортировать PNG.

## Детерминированность, даты и пределы точности

В конфигурации сохраняется **`as_of`** — замороженный timestamp, ограничивающий даты и часы сегодняшнего дня. С одинаковой полной конфигурацией, seed и snapshot результат побайтно идентичен. При обычных повторных запусках сохранённый cutoff остаётся прежним. `--days N` либо явный `--as-of now` обновляет его для нового плана; сохранённые планы это не меняет.

Даты включительны. Используются IANA timezone, независимый от DST обход календарных дат и точные смещения RFC3339. Timestamps отсортированы по реальным instant, включая переходы DST; переход в полночь также учитывается. Время после `as_of` не генерируется; если текущая дата только началась и доступно мало уникальных секунд, план отражает доступное количество. Git хранит timezone offset в author/committer date; календарные дни группируются в заданном timezone. Исторически несуществующая дата (например, 2011-12-30 в Pacific/Apia) отклоняется до генерации с объяснением. При добавлении нового плана проверяется реальная хронология с учётом разных timezone.

Projected уровни — квартили положительных ежедневных значений; это оценка относительной интенсивности. Точные внутренние пороги будущего GitHub, палитра текущей темы, правила атрибуции и задержки индексации могут отличаться. Полного совпадения цветов утилита не обещает. CURRENT сохраняет `color` и `contributionLevel`, полученные от API.

Защитные пределы: даты с 1970 года, до 36 600 суток в плане, до 500 новых/целевых коммитов за день, до 200 000 коммитов в плане. Большие планы стоит сначала просмотреть и сохранить. Публикация не изменяет глобальный Git config и не запускает hooks в сгенерированном репозитории. Сервер проверяет Host/Origin, использует случайный токен сессии, CSP и ограничение размера запросов.

## Разработка и проверки

```sh
make build
go test ./...
make test    # race detector; integration tests use only temporary local Git repos
make lint    # gofmt, go vet; goimports when available
make release # macOS arm64, Linux amd64, Linux arm64
```

Тесты покрывают seed, корреляцию, пресеты, лимиты, выходные/отдых/отпуска, timezone/DST, будущие timestamps, merging, checksum/serialization, реальные Git dates и identities, повторы/частичное восстановление, посторонние изменения, guarded HTTP API, head-bound approval, GraphQL и token fallback, публикацию во временный bare Git remote и CLI/wizard. Настоящий GitHub в тестах не используется.

Структура: `internal/forge` — модель/генератор; `render` — terminal/browser/exports; `gitstore` — owned history/verification; `github` — API/approval-bound push; `web` — локальная сессия; `cli` — команды/мастер. Все HTML/CSS/JS assets встроены в единственный бинарный файл.

В `examples/` лежат готовый Active Developer план на 365 дней, HTML/PNG/SVG/CSV, проверка 2 293 реальных локальных коммитов, screenshot браузера, запись терминального мастера и результаты тестов. Identity примера — `Activity Forge Demo / demo@example.invalid`: она предназначена для локальной демонстрации; для своей публикации создайте новый план со своим GitHub email. `SHA256SUMS` содержит контрольные суммы бинарников. Сборки Linux проверены кросс-компиляцией; интерактивная проверка выполнена на macOS arm64.
