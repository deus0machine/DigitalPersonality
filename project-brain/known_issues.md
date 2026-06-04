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
**Решение**: не планируется — анонимные посты не являются личными сообщениями.

### KI-002 — Удалённые аккаунты
**Проблема**: пользователь удалил аккаунт → его данные в Telegram API возвращают пустые поля.
**Текущее поведение**: имя может быть пустым, ID сохраняется.
**Риск**: ChatTitle в MessageHit может быть пустым.
**Решение**: COALESCE в запросах, пустая строка как default.

### KI-003 — RESOLVED: Flood wait (420)
**Была проблема**: `MessagesGetHistory` при FLOOD_WAIT помечал весь dialog как failed.
**Решение (2026-05-30)**:
- `fetchHistoryWithRetry` — retry с exponential backoff
- `tgerr.AsFloodWait(err)` — корректное определение FLOOD_WAIT
- Проактивный throttle: `SYNC_HISTORY_REQUEST_DELAY=200ms`
- ADR-0006 описывает полное решение

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
**Проблема**: код маппит megagroup в ChatTypeSupergroup, scoring одинаков.
**Риск**: активные megagroup получают тот же score что и маленькие группы.
**Решение**: можно добавить member count сигнал — отложено.

---

## MTProto Limitations

### KI-007 — Session persistence
**Проблема**: session файл должен быть защищён (mode 0600), chmod применяется вручную.
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
**Решение**: можно добавить blocklist по username — отложено.

### KI-010 — Broadcast channels без участия
**Текущее поведение**: score 0.10 — ниже порога 0.35, не синхронизируются.
**Риск**: каналы где пользователь участвует (комментарии) будут пропущены.
**Решение**: нет простого способа определить участие через ListDialogs — известное ограничение.

### KI-011 — Группы с низкой активностью пользователя
**Проблема**: group member score 0.60 не учитывает реальную активность пользователя.
**Риск**: много входящих сообщений без исходящих.
**Решение**: post-sync фильтрация по outgoing_ratio — отложено.

---

## Retrieval Limitations

### KI-012 — FTS только 'simple' dictionary
**Проблема**: `to_tsvector('simple', ...)` не делает стемминг для русского языка.
**Текущее поведение**: поиск точный по словам (без морфологии).
**Риск**: "свободен" не найдёт "свободного".
**Решение**: `pg_catalog.russian` конфигурация — Phase 5+.

### KI-013 — TopEmoji и TopSlang не реализованы
**Проблема**: поля `TopEmoji`, `TopSlang` в PersonalityReport всегда пустые.
**Текущее поведение**: инициализируются как пустые map, агрегация не написана.
**Решение**: SQL агрегация по emoji через regexp — следующая итерация.

### KI-014 — EpisodeHit без ChatTitle
**Проблема**: при поиске эпизодов ChatTitle может быть пустым если чат не уперсертирован.
**Текущее поведение**: пустая строка в этом случае.
**Решение**: COALESCE('', 'Unknown') в запросе.

---

## CLI Delivery

### KI-017 — TopEmoji и TopSlang всегда пустые в CLI personality
**Проблема**: `PersonalityReport.TopEmoji` и `TopSlang` пусты (KI-013).
**Риск**: пользователь не видит emoji/slang аналитику.
**Решение**: см. KI-013.

### KI-018 — personality без аргументов может быть длинным
**Проблема**: при большом числе чатов вывод длинный.
**Текущее поведение**: компактная таблица (одна строка на чат) — намеренно.
**Решение**: добавить `--limit N` при необходимости.

---

## Memory Window Architecture

### KI-019 — Orphan semantic docs (non-critical)
**Проблема**: при первом `ComputeParticipationWindows` часть `message_semantic` записей
остаётся для сообщений с `in_memory_window = FALSE`.
**Текущее поведение**: orphan docs не попадают в retrieval и не embed'ятся — корректность не нарушена.
**Риск**: небольшое лишнее место в `message_semantic`.
**Проверка**: V5 в `docs/sql/inspect_windows.sql`.
**Решение**: `DELETE FROM message_semantic WHERE message_id IN (SELECT id FROM messages WHERE NOT in_memory_window)` — при необходимости.

### KI-020 — Personality signals для out-of-window сообщений (исторические)
**Проблема**: до windowing все сообщения проходили Layer 3 — старые сигналы остаются.
**Риск**: небольшое искажение personality аналитики для social чатов.
**Проверка**: V4 в `docs/sql/inspect_windows.sql`.
**Решение**: DELETE при необходимости.

### KI-021 — Window computation не запускается для passive_consumption (score < threshold)
**Проблема**: broadcast каналы (score 0.10) не синхронизируются, ComputeAndRebuild не вызывается.
**Риск**: если такой чат был синхронизирован вручную — его сообщения идут в retrieval без windowing.
**Решение**: V7 в SQL toolkit покажет такие чаты. SyncThreshold для passive_consumption — отложено.

---

## Voice Transcription

### KI-022 — RESOLVED: access_hash not stored
**Была проблема**: невозможно построить InputPeer после перезапуска для private/channel/supergroup.
**Решение (migration 000007, Phase 5.1)**:
- `ALTER TABLE chats ADD COLUMN access_hash BIGINT NOT NULL DEFAULT 0`
- `entity.Chat.AccessHash int64`
- `ChatRepository.Upsert` сохраняет access_hash
- `sync/engine.go` пробрасывает `s.dialog.AccessHash`

### KI-023 — RESOLVED: transcribed_at отсутствует в message_semantic
**Была проблема**: нет idempotent checkpoint для transcription worker.
**Решение (migration 000007, Phase 5.1)**:
- `ALTER TABLE message_semantic ADD COLUMN transcribed_at TIMESTAMPTZ`
- Worker queue: `WHERE transcribed_at IS NULL`

---

## Utterance Embedding Pipeline (Phase 5.3)

### KI-024 — Staleness граничных utterances после sync
**Проблема**: при добавлении нового сообщения, которое попадает в gap последнего utterance чата,
тот же `first_message_id` начинает представлять другой текст (добавилось новое сообщение в группу).
Хранимый вектор остаётся старым, `FilterUnembedded` видит ID как уже embedded — не обновляет.

**Пример**:
```
До sync:  {A(10:00), B(10:02)} → utterance, first_message_id=A.id, вектор V1
После:    C(10:03) от того же автора → utterance теперь {A, B, C}
          V1 содержит text(A)+text(B), должен содержать text(A)+text(B)+text(C)
```

**Масштаб**: bounded числом активных чатов (сотни, не тысячи utterances одновременно).
**Риск**: low — незначительное искажение retrieval для самых свежих utterances.
**Решение**: принято как known limitation для MVP. Полное исправление требует материализации utterances
с `text_hash` колонкой для детекции изменений.
**Триггер для решения**: доля stale utterances > 5% при аудите качества retrieval.

### KI-025 — embed-utterances пересобирает весь корпус при каждом запуске
**Проблема**: `FetchAllInWindowMessages` → `Build()` загружает все 480k сообщений и строит
207k utterances каждый раз, даже если после sync добавилось 500 новых сообщений.
**Текущее поведение**: время запуска ~5-10 сек при 480k сообщениях, ~100-200MB RAM.
**Риск**: при corpus > 1M сообщений время вырастет до 30-60 сек, RAM до 500MB+.
**Решение**: принято для MVP (текущий масштаб приемлем). Инкрементальный подход потребует
либо материализации utterances, либо sync cursor для embeddings.
**Триггер для решения**: corpus > 1M сообщений или rebuild > 30 сек.

### KI-026 — HNSW индекс не поддерживает фильтрацию по gap/model
**Проблема**: pgvector HNSW ANN не умеет эффективно фильтровать по `WHERE gap_seconds = N`
до поиска. При наличии embeddings с разными gap значениями `SearchByVector` вернёт
смешанные результаты. Partial index решает проблему только для одной конфигурации.
**Текущее поведение**: таблица содержит embeddings только одного gap (текущего). Проблема
не проявляется пока gap не менялся.
**Риск**: если gap изменится без TRUNCATE, vector search вернёт stale embeddings без ошибки.
**Решение для MVP**: при смене `UTTERANCE_GAP_SECONDS` выполнить:
```sql
DELETE FROM utterance_embeddings;
```
затем re-run `embed-utterances`. `StoredGapSeconds()` в worker детектирует расхождение и падает с ошибкой.
**Триггер для изменения архитектуры**: появление второй embedding модели или второго gap параметра.

### KI-027 — Utterances не материализованы в БД
**Проблема**: utterances — runtime DTOs, не имеют стабильных DB-идентификаторов.
`first_message_id` используется как суррогатный ключ, но не является FK к таблице utterances
(такой таблицы нет). Нельзя делать join'ы, нельзя строить cross-source queries.
**Текущее поведение**: для Phase 5.3 (Telegram-only, single gap) достаточно.
**Риск**: при добавлении второго источника памяти (email, GitHub) потребуются stable utterance IDs.
**Решение**: материализация через `utterances` + `utterance_messages` таблицы — Phase 5.x или при
появлении второго источника. `text_hash` решит также KI-024.

### KI-028 — min_tokens фильтр (approx runes/4) может давать ложные срабатывания
**Проблема**: `len([]rune(text))/4 < 10` — аппроксимация. Для русского текста
1 руна ≈ 1-2 токена (cl100k_base), не 0.25 токена. Реальный порог ~10 токенов
соответствует ~40-80 рунам, а не 40 рунам как рассчитывает формула.
**Текущее поведение**: фильтр скипает utterances с `len(runes) < 40`. Для русского текста
это примерно 5-8 слов. Часть коротких, но семантически ценных utterances может скипаться.
**Риск**: low — utterances с 5-8 русскими словами редко несут уникальный personality сигнал.
**Решение**: добавить tiktoken-совместимый счётчик или увеличить порог до `len(runes) < 20`
(~3-4 русских слова) для более точного фильтра. Не критично для MVP.

### KI-029 — Latency query embedding для интерактивного use case
**Проблема**: каждый вызов `retrieve-vector` делает HTTP запрос к OpenAI API для embed query
(100-300ms). Для CLI это приемлемо. Для HTTP API (Phase 6) с ожидаемым SLA < 500ms это blocker.
**Текущее поведение**: `retrieve-vector` — CLI batch command, latency не критична.
**Риск**: при добавлении HTTP API в Phase 6 потребуется либо local embedding model,
либо query embedding cache.
**Решение**: отложено до Phase 6. Зафиксировать при проектировании HTTP API.

---

## Architectural Concerns

### KI-015 — app.go вырастает
**Проблема**: `internal/app/app.go` содержит всю сборку зависимостей линейно.
**Риск**: при добавлении HTTP server (Phase 6) станет God Object.
**Решение**: разбить на подсборки (`buildSyncEngine`, `buildRetrievalStack`) — при необходимости.

### KI-016 — Нет HTTP delivery layer
**Текущее состояние**: `retrieval.Service` подключён только к CLI.
**Решение**: Phase 6 добавит HTTP endpoint поверх того же retrieval.Service.
