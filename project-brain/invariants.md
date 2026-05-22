# Архитектурные инварианты

Это жёсткие ограничения. Нарушение любого из них — архитектурный регресс.

---

## I1 — Зависимости только внутрь

```
infrastructure → application → domain
```

Domain не знает application. Application не знает infrastructure.
Нарушение: импорт `pgx`, `tg.*` в domain или application пакетах.

---

## I2 — Telegram-типы не выходят из infrastructure

`gotd/td` типы (`tg.Message`, `tg.User`, `tg.Dialog` и т.д.) существуют только внутри
`internal/infrastructure/telegram/`.

На границе: `internal/application/port/telegram.go` — чистые Go-структуры без зависимостей.
Нарушение: `tg.*` в application, domain, или interfaces пакетах.

---

## I3 — Raw messages immutable

Таблица `messages` — источник правды. После upsert'а данные не удаляются и не перезаписываются деструктивно.
`is_deleted = TRUE` — единственный способ «удалить».
Semantic, personality, episode слои строятся поверх и независимо.
Нарушение: DELETE FROM messages, изменение sent_at / text после записи.

---

## I4 — Inspectability важнее premature optimization

Все scoring-решения и состояния должны быть видны через SQL без кода.
Пример: все чаты хранятся с `relevance_score + reason + surface`, даже если они ниже порога.
Нарушение: хранить только «нужные» чаты, терять причины scoring-решений.

---

## I5 — Scorer — pure function

`ChatRelevanceScorer.Score(d DialogInfo) ChatRelevance` — детерминирована, без I/O, без state.
Нарушение: запросы в БД внутри Score(), глобальный state, side effects.

---

## I6 — Retrieval layer explainable

Каждый результат поиска несёт `MatchType` ("fts" | "trigram" | "filter") и `Rank`.
Нарушение: возвращать результаты без объяснения источника ранжирования.

---

## I7 — Personality extraction не теряет verbatim data

`messages.text` хранится as-is: emoji, капс, пунктуация, язык — без изменений.
Нормализация происходит только в `message_semantic`, никогда не перезаписывает raw.
Нарушение: очищать/нормализовать text перед записью в messages.

---

## I8 — Embeddings — infrastructure, не business logic

Векторные представления — деталь реализации retrieval.
`application/retrieval/repository.go` может добавить `SearchByVector`,
но сам вектор не должен появляться в domain entities или application use cases напрямую.
Нарушение: `[]float32` в entity.Message, в Query, в domain интерфейсах.

---

## I9 — Context propagation везде

Каждый публичный метод, который делает I/O, принимает `context.Context` первым параметром.
Нарушение: методы репозиториев без ctx, background goroutines без ctx.

---

## I10 — Secrets только из environment

Telegram API ID/Hash, OpenAI key, DSN — только из env vars.
Никаких хардкодов, никаких конфиг-файлов с секретами в репозитории.
`.env` в `.gitignore`.
Нарушение: ключи в коде, в committed конфиг-файлах.

---

## I11 — Одна миграция — одно изменение схемы

Каждый `migrations/NNNNNN_name.up.sql` — атомарное изменение.
Никогда не редактировать применённые миграции.
Нарушение: изменение существующего up.sql после его применения в prod/dev.

---

## I12 — Бизнес-логика не в handlers/controllers

Delivery layer (HTTP, CLI) только: парсит запрос → вызывает use case → форматирует ответ.
Нарушение: фильтрация, scoring, агрегация в HTTP handlers.
