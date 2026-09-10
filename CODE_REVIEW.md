# Code Review: ImageServer

> Обзор и оценка недостатков кодовой базы проекта `ImageServer` (Go).
> Дата: 2026-08-22 · Ревизия выполнена по исходному коду каталога `src/`.

---

## 1. Обзор проекта

Сервер изображений для фоторамки AIFrames. Получает запросы от рамки, отдаёт картинки. Источники изображений — внешние провайдеры (адаптер YandexArt) и локальное хранилище. Поддерживает «промпты» с плейсхолдерами, метрики, периоды сна, REST/WEB интерфейс, SQLite (SQLite через `modernc.org/sqlite`) с миграциями goose.

### Состав модулей

| Пакет | Назначение |
|---|---|
| `appimgserver` | Сборка приложения, запуск, graceful shutdown |
| `rest` | HTTP/TLS сервер, REST API, WEB UI |
| `opermanager` | Очередь операций, статусы, выбор провайдера |
| `promptmanager` | Промпты, плейсхолдеры, миграция `idx → code`, статистика |
| `templater` | Замена плейсхолдеров `[[name]]` |
| `ydart` / `localimageprovider` | Адаптеры провайдеров |
| `dirmanager` | Список файлов, лимиты, очистка |
| `imageprocessor` | Масштабирование/конвертация изображений |
| `metrics` | Метрики (go-metrics) |
| `dbase` | Инициализация SQLite, миграции, бэкапы, статистика промптов |
| `actioner` / `timerange` / `utils` / `helpers` / `mylogger` | Утилиты |

---

## 2. Критические ошибки (баги)

### 2.1 Паника при единственном промпте — `GetRandomPromptValue`
**ИСПРАВЛЕНО**

[`promptmanager.go:127`](src/internal/pkg/promptmanager/promptmanager.go:127)

```go
if keysCount == 1 {
    return pm.GetPromptValue(pm.prompts[pm.promptKeys[1]]), nil // индекс [1] вне диапазона
}
```

При `keysCount == 1` слайс `promptKeys` имеет длину 1, обращение к `promptKeys[1]` вызывает **panic (index out of range)**. Это штатный сценарий — при первом запуске создаётся файл с одним промптом `prmt_1` ([`createDefaultPrompts`](src/internal/pkg/promptmanager/promptmanager.go:574)).

### 2.2 Случайный выбор промпта сломан после миграции `idx → code`
**ИСПРАВЛЕНО**

[`promptmanager.go:131`](src/internal/pkg/promptmanager/promptmanager.go:131)

```go
randomIndex := pm.rng.Intn(keysCount) + 1 // +1, так как ключи начинаются с 1
value, exists := pm.prompts[pm.promptKeys[randomIndex]]
```

Логика рассчитана на то, что ключи — целые `1..N`. После перехода на `code` (строки вида `prmt_1`, `test1`, пользовательские) ключи сортируются лексикографически, а не числовые. Выбор `randomIndex` из диапазона `1..N` теперь указывает на **произвольные** элементы или вовсе выходит за границы, и существующие промпты могут не выбираться вообще (цикл из 100 попыток завершится ошибкой). Это прямое следствие задачи «Change prompt index to prompt code (#30)».

### 2.3 Ошибка запуска HTTP-сервера проглатывается
**ИСПРАВЛЕНО**

[`imgserver.go:314`](src/internal/appimgserver/imgserver.go:314)

```go
err = app.restObj.Start()
app.logger.Error("Error start rest", "error", err)
return nil
```

`Start()` всегда возвращает `nil`. Если сертификаты отсутствуют (`/certs/*`), `restObj.Start()` вернёт ошибку, но она лишь логируется — приложение продолжает «работать», не имея HTTP-интерфейса. `main.go` не увидит ошибку, т.к. `err == nil`.

### 2.4 Nil-указатель в провайдере Lim при пустом `local_image_folder`

**Исправлено**

[`localimageprovider/lim.go:135`](src/internal/pkg/localimageprovider/lim.go:135)

Если `local_image_folder` не задан, `dm == nil`. При этом `Refresh()` вызывается планировщиком по расписанию ([`imgserver.go:290`](src/internal/appimgserver/imgserver.go:290)):

```go
func (lim *Lim) Refresh() error { return lim.dm.ReadFiles() } // panic при dm == nil
```

Аналогично `GetImageSlice()` вызывает `lim.dm.GetRandomFile()` без проверки на `nil` — падение в горутине (крэш процесса или silent panic в зависимости от контекста).

### 2.5 Коллизии идентификаторов и имён файлов (по секундам)
**Исправлено**

[`opermanager.go:594`](src/internal/pkg/opermanager/opermanager.go:594)

```go
func (op *OperMngr) generateId() string {
    unixSeconds := time.Now().Unix()
    return "i" + strconv.Itoa(int(unixSeconds))
}
```

Идентификатор операции строится из Unix-времени в секундах. При нескольких операциях в одну секунду ID совпадают и перезаписывают друг друга в `pendingOperations`. Та же проблема в `generateFileName`/`generateTemporaryFileName` — файлы в одной секунде перезаписываются (`i`/`f` префикс одинаков, суффикс `-orig`/`fN.jpeg`).

---

## 3. Проблемы безопасности

| № | Проблема | Место |
|---|---|---|
| 3.1 | **Нет аутентификации/авторизации** ни на одном REST-эндпоинте. Сервер генерирует платные изображения (YandexArt) и позволяет удалять/редактировать промпты. Доступен любому, кто имеет сетевой доступ. | [`rest.go:64`](src/internal/pkg/rest/rest.go:64) |
| 3.2 | **Потенциальный XSS** через `template.JS` (`jsonify`). Промпты — пользовательские данные, попадают в HTML/JS как `template.JS` без экранирования. | [`rest.go:903`](src/internal/pkg/rest/rest.go:903) |
| 3.3 | `API key` YandexArt хранится открытым текстом в JSON и монтируется в контейнер. Для self-hosted приемлемо, но стоит зафиксировать в доке. | [`ydart.go:403`](src/internal/pkg/ydart/ydart.go:403) |
| 3.4 | Отсутствуют лимиты на размер/скорость запросов; нет `rate limiting` для платных генераций. | [`rest.go`](src/internal/pkg/rest/rest.go:64) |

---

## 4. Обработка ошибок и логирование

### 4.1 Игнорирование ошибок
- [`imgserver.go:363`](src/internal/appimgserver/imgserver.go:363): `plan, _ := os.ReadFile(FILE_PATH_OPTIONS)` — ошибка чтения конфига игнорируется, дальше `yaml.Unmarshal` на пустых данных.
- [`dbutils.go`](src/internal/pkg/dbase/dbutils.go): ошибки `scheduler.NewJob(...)` не проверяются (`_, err =`).
- [`dirmanager.go:233`](src/internal/pkg/dirmanager/dirmanager.go:233): ошибка `os.Remove` логируется, но `innerCleanUp` продолжает менять список — возможна рассинхронизация.
- [`opermanager.go:456`](src/internal/pkg/opermanager/opermanager.go:456): ошибка записи оригинала логируется, но поток продолжается.

### 4.2 Злоупотребление уровнем `Error`
Множество мест используют `logger.Error(...)` для **штатных** сообщений:
- [`imgserver.go:135`](src/internal/appimgserver/imgserver.go:135): `logger.Error("This is not error. Current options", ...)` — дамп опций на уровне Error.
- [`imgserver.go:314`](src/internal/appimgserver/imgserver.go:314): `app.logger.Error("Error start rest", "error", err)`.
- [`imgserver.go:337`](src/internal/appimgserver/imgserver.go:337): `app.logger.Error("SHUTDOWN")` — обычный вывод.
- [`rest.go:81`](src/internal/pkg/rest/rest.go:81): `logger.Error("(It is not error!!!) Run WEB-Server...")`.
- [`rest.go:521`](src/internal/pkg/rest/rest.go:521): `rest.logger.Error("***", ...)`.

Это портит мониторинг по уровням логирования и делает невозможным фильтрацию реальных ошибок.

### 4.3 `panic` вместо возврата ошибок
- [`imgserver.go:91,149,157,173,218`](src/internal/appimgserver/imgserver.go): стартовые ошибки выбрасываются через `panic(fmt.Sprintf(...))`. Лучше возвращать `error` и обрабатывать в `main`.
- [`ydart.go:94`](src/internal/pkg/ydart/ydart.go:94): `readSecretOptions` → `panic` при недоступности файла.

### 4.4 Стартовые «тестовые» сообщения в лог
[`imgserver.go:132-134`](src/internal/appimgserver/imgserver.go:132): при каждом старте пишутся Debug/Warn/Error «заглушки», засоряющие прод-логи.

---

## 5. Гонки и конкурентность

- **[`actioner.go:18`](src/internal/pkg/actioner/actioner.go:18)**: `lastCallTime` читается/пишется из нескольких горутин (обработчик `StartOperation` + планировщик `CheckPendingOperations`) **без синхронизации** — data race.
- [`opermanager.go:229`](src/internal/pkg/opermanager/opermanager.go:229): использование глобального `math/rand.Intn` из многих горутин.
- [`promptmanager.go:157`](src/internal/pkg/promptmanager/promptmanager.go:157): `templater.ReplacePlaceholders` использует глобальный `rand.Intn`.
- Очередь `nextGeneration` — единственный слот ([`opermanager.go:183`](src/internal/pkg/opermanager/opermanager.go:183)); при двух POST подряд второй перезаписывает первый (потеря операции). Для очереди ожидаешь FIFO-буфер, а не единичный слот.

---

## 6. Конфигурация и окружение

- **Пути захардкожены**: `/data/options.yml`, `/data/prompts.yaml`, `/data/ydart-options.json`, `/log/app.log`, `/certs/*`, `/images`. Код жёстко привязан к Docker-раскладке, вне контейнера не запускается.
- **Несоответствие `docker-compose.yml` и кода**: в compose задаётся `DB_PATH=/data/db/myapp.db`, но код использует жёсткий `DATABASE_PATH = "/data/db/app.dbase"` ([`dbutils.go:18`](src/internal/pkg/dbase/dbutils.go:18)). Переменная `DB_PATH` в коде **не читается**.
- **Расхождение `options.yml` и структуры `ApplOptions`**: в корневом [`options.yml`](options.yml:1) заданы плоские `image_weight`/`image_height`, тогда как структура ожидает вложенный `iframe_image_parameters.image_weight` и т.д. ([`imgserver.go:67`](src/internal/appimgserver/imgserver.go:67)), плюс отсутствует `prompts_amount`. Фактически конфиг не маппится корректно и используются дефолты.
- Миграции ищутся по относительному пути `./migrations` ([`dbutils.go:60`](src/internal/pkg/dbase/dbutils.go:60)) — зависит от рабочего каталога процесса.

---

## 7. Сеть и HTTP-клиент

- [`ydart.go:98`](src/internal/pkg/ydart/ydart.go:98): используется `http.DefaultClient` — **без таймаутов**, retry и контекста. Запрос к YandexArt может висеть бесконечно, блокируя горутину.
- Нет проброса `context`/отмены при shutdown.
- Семантика HTTP-кодов смешана: `422 UnprocessableEntity` используется и для клиентских, и для внутренних ошибок (правильнее `500`). Например [`rest.go:227`](src/internal/pkg/rest/rest.go:227).
- **ИСПРАВЛЕНО** [`rest.go:652`](src/internal/pkg/rest/rest.go:652): `json.NewEncoder(w).Encode(...)` затем `w.WriteHeader(http.StatusOK)` — `WriteHeader` после записи тела является ошибкой (заголовки уже отправлены), вызов бесполезен/вводит в заблуждение.
- **ИСПРАВЛЕНО** Дублирование API: `/prompt/add` ([`rest.go:79`](src/internal/pkg/rest/rest.go:79)) и `/api/prompts` ([`rest.go:68`](src/internal/pkg/rest/rest.go:68)) реализуют одно и то же по-разному (разные форматы кода: `2006_01_02_15_04_05` против `06_01_02_15_04_05`).

---

## 8. Хранение и работа с файлами

- **ИСПРАВЛЕНО** Запись `prompts.yaml` не атомарная (пишется напрямую в конечный файл без temp+rename, [`promptmanager.go:462`](src/internal/pkg/promptmanager/promptmanager.go:462)) — при сбое во время записи файл будет повреждён.
- Промпты хранятся в файле, а статистика — в БД: рассинхронизация двух источников истины (например, `DeletePrompt` удаляет и то и другое, но не транзакционно).
- `pendingOperations`/`completeOperations` хранятся в памяти (`go-cache`) — при рестарте зависшие операции у провайдера теряются.
- В [`rest.go:266`](src/internal/pkg/rest/rest.go:266) файл целиком читается в память и кодируется в base64 — память O(N) от размера изображения.

---

## 9. Качество кода и дублирование

- Много мёртвого/закомментированного кода: [`ydart.go:360-401`](src/internal/pkg/ydart/ydart.go:360) (закомментированный `processImage`), [`main.go:34`](src/main.go:34) (`//app.Start()`), [`rest.go:116-117`](src/internal/pkg/rest/rest.go:116).
- Дублирование обработчиков промптов (см. п.7), дублирование логики периодов сна в `opermanager` и `ydart`.
- [`ipr.go:86`](src/internal/pkg/imageprocessor/ipr.go:86): сам автор отметил TODO о повторяющемся коде в `ConvertImageFileToJpg`/`ConvertBase64ToJpg`.
- Смешение языков в сообщениях и комментариях (русский/английский), комментированный отладочный вывод.
- Структуры и поля не всегда имеют `json`/`yaml` теги (например, `PromptCardResponse`).
- **ИСПРАВЛЕНО** Использование `fmt.Errorf("...%v...")` вместо `%w` для обёртки ошибок в ряде мест.

---

## 10. Тестирование

Покрытие крайне малое и не затрагивает ключевую логику:

- Существуют тесты: `StatisticDao_test.go`, `dirmanager_test.go`, `ipr_test.go`, `templater_test.go`.
- **Нет тестов** для самого багового места — случайного выбора промпта `GetRandomPromptValue` (где присутствует паника и сломанный индекс), а также для `opermanager`, `rest`, `ydart`, `lim` (nil-указатели), `promptmanager` (миграция `idx → code`).
- Нет интеграционных тестов REST-эндпоинтов, нет тестов graceful shutdown.

---

## 11. БД и статистика промптов (задачи #29/#30)

Частично реализовано, но есть недоработки:

- Таблица `template_statistic` ([`000001_initial.sql`](src/migrations/000001_initial.sql:2)) и `StatisticDao` ([`StatisticDao.go`](src/internal/pkg/dbase/StatisticDao.go:18)) реализованы.
- `Increment` выполняет два запроса (UPDATE, затем INSERT-фолбэк) — можно заменить одним UPSERT. При `SetMaxOpenConns(1)` ([`dbutils.go:52`](src/internal/pkg/dbase/dbutils.go:52)) все инкременты сериализуются — потенциальное узкое место.
- Счётчик инкрементируется внутри `GetPromptValue` ([`promptmanager.go:148`](src/internal/pkg/promptmanager/promptmanager.go:148)) — то есть статистика учитывает **и подготовку** значения, и генерацию. Надо решить, считать ли только фактически отправленные провайдеру промпты.
- После миграции `idx → code` ключом статистики стал строковый `code`, но алгоритм случайного выбора всё ещё работает с числовым индексом (см. п.2.2) — статистика корректна, только пока коды совпадают с `prmt_N` последовательно.
- `GetStatistic` делает полный запрос `SELECT *` при каждом рендере страницы — для больших объёмов лучше агрегировать на стороне запроса.

---

## 12. Сводка приоритетов

### 🔴 Высокий приоритет (исправить в первую очередь)
1. Паника в `GetRandomPromptValue` при 1 промпте (п.2.1).
2. Сломанный случайный выбор промптов после перехода на `code` (п.2.2).
3. Проглатывание ошибки запуска HTTP-сервера (п.2.3).
4. Nil-указатель в `Lim.Refresh`/`GetImageSlice` из планировщика (п.2.4).
5. Коллизии ID/имён файлов по секундам (п.2.5).
6. Отсутствие таймаутов у `http.DefaultClient` (п.7).
7. Нет аутентификации на REST API (п.3.1).

### 🟡 Средний приоритет
8. Data race в `Actioner.lastCallTime` (п.5).
9. `panic` вместо возврата ошибок при старте (п.4.3).
10. Злоупотребление уровнем `Error` в логах (п.4.2).
11. Рассинхронизация конфигов: `options.yml`, `DB_PATH`, пути миграций (п.6).
12. Потеря `pending` операций при рестарте; единичный слот очереди (п.5, п.8).
13. Неатомарная запись `prompts.yaml` (п.8).

### 🟢 Низкий приоритет
14. Мёртвый/закомментированный код, дублирование (п.9).
15. Малое тестовое покрытие ключевой логики (п.10).
16. Смешение HTTP-кодов 422/500, `WriteHeader` после `Encode` (п.7).
17. Семантика статистики промптов и оптимизация `Increment`/`GetAll` (п.11).

---

## 13. Рекомендации

1. **Переписать выбор промпта** на работу со строковыми кодами: собрать `[]string` кодов и выбирать `keys[rand.Intn(len(keys))]`, убрать ветку с `keysCount == 1` или корректно обработать её.
2. **Возвращать ошибки из `ImgSrv.Start()`** и завершать приложение (Fatal) при неудачном старте HTTP-сервера.
3. Добавить **`nil`-проверки** для `Lim.dm` в `Refresh` и `GetImageSlice`.
4. Перейти на **UUID/уникальные ID** для операций и имён файлов вместо Unix-секунд.
5. Настроить **`http.Client` с таймаутами** и пробросом `context`.
6. Добавить **авторизацию/токен** хотя бы для мутирующих эндпоинтов и ограничить доступ.
7. Вынести конфигурацию путей в env/опции вместо хардкода.
8. Добавить **unit-тесты** для `promptmanager` (случайный выбор, миграция), `lim` (nil-кейсы), `opermanager` (идентификаторы, статусы), REST-обработчиков.
9. Использовать `logger.Info/Debug` для штатных событий, `Error` — только для фактических ошибок.
10. Устранить data race в `Actioner` (мутекс/atomic).