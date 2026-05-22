# ADR-0003 — Personality Surfaces

_Дата: 2026-05-22 | Статус: принято_

## Контекст

Разные типы диалогов содержат разные виды personality данных.
Сообщение в Saved Messages — это внутренний монолог.
Сообщение в групповом чате — это социальное поведение.
Сообщение боту — это инструментальное взаимодействие.

Если смешивать всё в одну кучу, retrieval будет возвращать нерелевантный контекст
при LLM-симуляции (Phase 6): стиль общения с ботом ≠ стиль общения с другом.

## Решение

`PersonalitySurface` — tagged union из 5 значений:

| Surface | Примеры | Что отражает |
|---------|---------|--------------|
| `self_expression` | Saved Messages, собственные каналы | Внутренний голос, авторский контент |
| `interpersonal` | Private 1:1 чаты | Личное общение, отношения |
| `social` | Группы, supergroups | Публичное поведение в сообществе |
| `tool_interaction` | Боты, сервисы | Инструментальный стиль |
| `passive_consumption` | Чужие broadcast каналы | Вкусы, интересы (только входящие) |

Surface присваивается на уровне чата (`chats.personality_surface`), не сообщения.
Это упрощение: технически один чат = одна поверхность.

## Альтернативы

**Per-message surface**: точнее, но требует inference на каждом сообщении.
Слишком дорого для Phase 4, откладываем.

**Flat label без типизации**: просто строка в БД. Отвергнуто: нет compile-time safety,
нет exhaustive matching в scorer.

**Больше поверхностей** (например отдельно `professional`, `romantic`):
Premature: недостаточно данных для валидации. Можно добавить позже.

## Tradeoffs

**Плюсы**:
- Retrieval может фильтровать по surface: `SearchMessages(ctx, Query{Surface: SurfaceInterpersonal})`
- Phase 6 сможет выбирать контекст релевантный запросу
- Scorer присваивает surface без I/O — pure function

**Минусы**:
- Один чат = одна surface — упрощение
- Группа может быть и social и professional — нет способа выразить это
- passive_consumption почти никогда не синхронизируется (score 0.10 < threshold 0.35)

## Последствия

- `entity.PersonalitySurface` в `domain/entity/chat.go`
- `chats.personality_surface TEXT` — хранится как строка, не enum (проще миграции)
- `Query.Surface` в retrieval позволяет фильтровать результаты по поверхности
- При добавлении новой поверхности: добавить константу → обновить scorer → новая миграция не нужна
