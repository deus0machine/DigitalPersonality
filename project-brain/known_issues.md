# Known Issues

---

## Telegram Edge Cases

### KI-000 — RESOLVED: sender_id FK violation
**Была проблема**: `messages_sender_id_fkey` нарушалась для участников групп и peer в личных чатах.
**Решение (2026-05-27)**:
- `HistoryPage.Participants []UserInfo` — извлекаем `v.Users` из каждого API response
- `upsertParticipants()` — upsert всех участников страницы до обработки сообщений
- `UserRepository.EnsureExists()` — fallback `INSERT ... ON CONFLICT DO NOTHING` для edge cases
- `mapParticipants()` — фильтрует `*tg.UserEmpty` (deleted accounts)

---

### KI-001 — SenderID = 0 для анонимных постов
**Проблема**: в каналах и некоторых группах `msg.FromID` отсутствует. `resolveSenderID` возвращает 0.
**Текущее поведение**: sender_id хранится как NULL (через nullInt64(0)).
**Риск**: агрегация по sender_id исключает эти сообщения.
**Решение**: не планируется пока — анонимные посты не являются личными сообщениями.

### KI-002 — Удалённые аккаунты
**Проблема**: пользователь удалил аккаунт → его данные в Telegram API возвращают пустые поля.
**Текущее поведение**: имя может быть пустым, ID сохраняется.
**Риск**: ChatTitle в MessageHit может быть пустым.
**Решение**: COALESCE в запросах, пустая строка как default.

### KI-003 — RESOLVED: Flood wait (420)
**Была проблема**: `MessagesGetHistory` при FLOOD_WAIT помечал весь dialog как failed.
Большие ценные диалоги пропускались полностью, backfill становился неполным.
**Решение (2026-05-30)**:
- `fetchHistoryWithRetry` в `telegram/client.go` — retry с exponential backoff
- `tgerr.AsFloodWait(err)` — правильное определение FLOOD_WAIT (не как ошибки бизнес-логики)
- Формула sleep: `floodWait × multiplier^(attempt-1) + jitter`
- Лог WARN до исчерпания retries, лог ERROR только при окончательном провале
- Проактивный throttle: `SYNC_HISTORY_REQUEST_DELAY=200ms` между page requests
- Конфиг: `SYNC_FLOOD_MAX_RETRIES`, `SYNC_FLOOD_JITTER`, `SYNC_FLOOD_BACKOFF_MULT`
- ADR-0006 описывает полное решение и trade-offs

### KI-004 — Sticker set name
**Проблема**: `DocumentAttributeSticker.Stickerset` возвращает InputStickerSetID, не имя.
**Текущее поведение**: `StickerInfo.SetName` всегда пустой.
**Риск**: нельзя группировать стикеры по паку.
**Решение**: требует отдельного API call `messages.getStickerSet` — отложено.

### KI-005 — Custom emoji в реакциях
**Проблема**: `ReactionCustomEmoji` хранит DocumentID, не unicode символ.
**Текущее поведение**: `"custom:" + string(rune(v.DocumentID))` — нечитаемо.
**Риск**: TopEmoji аналитика будет засорена custom emoji ID.
**Решение**: разрешение требует API call — отложено до Phase 5+.

### KI-006 — Megagroup vs Supergroup
**Проблема**: Telegram называет большие группы "megagroup" (ch.Megagroup = true), код маппит в ChatTypeSupergroup.
**Текущее поведение**: корректно, но scoring для supergroup и group одинаков.
**Риск**: активные megagroup могут получить тот же score что и маленькие группы.
**Решение**: можно добавить member count сигнал — отложено.

---

## MTProto Limitations

### KI-007 — Session persistence
**Проблема**: session файл (`*.session`) должен быть защищён (mode 0600).
**Текущее поведение**: клиент создаёт файл, но chmod применяется в os-level вручную.
**Риск**: в Docker/Linux разрешения могут наследоваться неправильно.
**Решение**: проверять разрешения при старте — не реализовано.

### KI-008 — GetHistory возвращает сообщения newest-first
**Текущее поведение**: пагинация через OffsetID по убыванию ID — корректно.
**Риск**: если новые сообщения приходят во время backfill, они будут пропущены.
**Решение**: sync cursor сохраняет maxIDSeen — при следующем запуске догонит.

---

## Scoring Edge Cases

### KI-009 — Ложноположительные для bot диалогов
**Проблема**: score 0.50 для ботов — возможно слишком высоко для spam/service ботов.
**Текущее поведение**: все боты синхронизируются (выше SyncThreshold 0.35).
**Риск**: засорение памяти бесполезными bot-диалогами.
**Решение**: можно добавить blocklist по username или снизить threshold до 0.40+.

### KI-010 — Broadcast channels без участия
**Текущее поведение**: score 0.10 — ниже порога 0.35, не синхронизируются.
**Риск**: каналы где пользователь участвует (комментарии) будут пропущены.
**Решение**: нет простого способа определить участие через ListDialogs — известное ограничение.

### KI-011 — Группы с низкой активностью пользователя
**Проблема**: group member score 0.60 не учитывает что пользователь может быть неактивен.
**Текущее поведение**: синхронизируются все группы выше порога.
**Риск**: много входящих сообщений без исходящих.
**Решение**: можно добавить post-sync фильтрацию по outgoing_ratio — отложено.

---

## Retrieval Limitations

### KI-012 — FTS только 'simple' dictionary
**Проблема**: `to_tsvector('simple', ...)` не делает стемминг для русского языка.
**Текущее поведение**: поиск точный по словам (без морфологии).
**Риск**: "свободен" не найдёт "свободного".
**Решение**: установить `postgresql-15-rum` или `pg_catalog.russian` конфигурацию — Phase 5+.

### KI-013 — TopEmoji и TopSlang не реализованы
**Проблема**: поля `TopEmoji`, `TopSlang` в PersonalityReport всегда пустые.
**Текущее поведение**: инициализируются как пустые map, агрегация не написана.
**Решение**: SQL агрегация по emoji через regexp — следующая итерация.

### KI-014 — EpisodeHit без ChatTitle
**Проблема**: при поиске эпизодов ChatTitle приходит из JOIN с chats — может быть пустым если чат не уперсертирован.
**Текущее поведение**: пустая строка в этом случае.
**Решение**: COALESCE('', 'Unknown') в запросе.

---

## CLI Delivery

### KI-017 — TopEmoji и TopSlang всегда пустые в CLI personality
**Проблема**: `PersonalityReport.TopEmoji` и `TopSlang` инициализируются пустыми картами (KI-013).
**Текущее поведение**: секции "Top Emoji" и "Top Slang Markers" не отображаются в выводе `personality`.
**Риск**: пользователь не видит emoji/slang аналитику.
**Решение**: см. KI-013 — SQL агрегация по personality_signals.

### KI-018 — personality без аргументов может быть длинным
**Проблема**: `personality` без chat-id выводит детальные отчёты для всех чатов.
**Текущее поведение**: компактная таблица (одна строка на чат) — это намеренно.
**Риск**: при большом числе чатов всё равно может быть длинный вывод.
**Решение**: добавить `--limit N` или пагинацию — при необходимости в Phase 5.

---

## Memory Window Architecture

### KI-019 — Orphan semantic docs (non-critical)
**Проблема**: при первом запуске `ComputeParticipationWindows` для social/passive_consumption чата,
некоторые сообщения получают `in_memory_window = FALSE`, но их записи в `message_semantic` остаются.
**Текущее поведение**: orphan docs не попадают в retrieval queries (filter `in_memory_window=TRUE`)
и не embed'ятся (`ListPendingEmbedding` теперь JOIN с messages + фильтр `in_memory_window`).
**Риск**: небольшое занятое место в `message_semantic`. Не влияет на корректность.
**Проверка**: V5 в `docs/sql/inspect_windows.sql` покажет count orphan docs.
**Решение**: плановая чистка `DELETE FROM message_semantic WHERE message_id IN (SELECT id FROM messages WHERE NOT in_memory_window)` — при необходимости.

### KI-020 — Personality signals для out-of-window сообщений (исторические)
**Проблема**: до windowing все сообщения проходили Layer 3. После первого compute,
сигналы для non-window сообщений остаются в `personality_signals`.
**Текущее поведение**: новые сигналы для non-window сообщений не создаются (sync/engine.go
не вызывает extractor для non-window messages после rebuild). Старые остаются.
**Риск**: небольшое искажение personality аналитики для social чатов.
**Проверка**: V4 в `docs/sql/inspect_windows.sql`.
**Решение**: при необходимости — `DELETE FROM personality_signals WHERE message_id IN (SELECT id FROM messages WHERE NOT in_memory_window)`.

### KI-021 — Window computation не запускается для passive_consumption (score < threshold)
**Проблема**: подписные broadcast каналы имеют score 0.10 — ниже SyncThreshold 0.35.
Они никогда не синхронизируются, поэтому `ComputeAndRebuild` для них не вызывается.
**Текущее поведение**: если чат когда-либо был синхронизирован (вручную или при изменении threshold),
его сообщения имеют `in_memory_window = TRUE` по default. Window computation не запускался.
**Риск**: passive_consumption чаты без windowing проходят через retrieval как будто full-sync.
**Решение**: можно снизить SyncThreshold для passive_consumption — отложено. V7 в SQL toolkit покажет такие чаты.

---

## Architectural Concerns

### KI-015 — app.go вырастает
**Проблема**: `internal/app/app.go` уже содержит всю сборку зависимостей линейно.
**Риск**: при добавлении Phase 5 (embedding worker, HTTP server) станет God Object.
**Решение**: разбить на подсборки (`buildSyncEngine`, `buildRetrievalStack`) — при необходимости.

### KI-016 — Нет HTTP delivery layer
**Проблема**: `retrieval.Service` подключён только к CLI, не к HTTP API.
**Текущее состояние**: CLI покрывает inspection use case, HTTP нужен для Phase 6 (LLM persona).
**Решение**: Phase 6 добавит HTTP endpoint поверх того же retrieval.Service.
