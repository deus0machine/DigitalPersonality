# ADR-0002 — Dialog Relevance Scoring (вместо бинарного фильтра)

_Дата: 2026-05-22 | Статус: принято_

## Контекст

Первая реализация использовала бинарный `DialogFilter`: include/exclude по типу чата.
При реальном тесте выяснилось: аккаунт содержит собственные Telegram-каналы.
Бинарный фильтр исключал broadcast channels — и собственные каналы тоже,
хотя они являются высокоценными personality данными (self_expression).

Проблема: бинарная логика не может выразить «свой канал важнее чужого».

## Решение

`ChatRelevanceScorer` — pure function, возвращает `ChatRelevance{Score float32, Reason string, Surface PersonalitySurface}`.

Шкала 0.0–1.0:
- Saved Messages: 1.00
- Собственный канал (creator): 0.90
- Собственный канал (admin): 0.75
- Private 1:1: 0.85
- Bot диалог: 0.50
- Группа/supergroup (creator/admin/member): 0.80/0.70/0.60
- Passive broadcast channel: 0.10

`SyncThreshold = 0.35` — ниже не синхронизируем.

**Ключевое**: ВСЕ чаты апсертируются в БД с оценками, даже если ниже порога.
Это означает инспектабельность через SQL без изменения кода.

## Альтернативы

**ML-based scoring**: слишком сложно для текущей фазы. Нет training data.

**Конфигурируемые пороги через env/config**: усложняет операционную модель.
Scorer правила закодированы явно и версионируются через git.

**Whitelist/blacklist по chat_id**: не масштабируется, требует ручного обслуживания.

## Tradeoffs

**Плюсы**:
- Чистая функция — детерминирована, легко тестируется
- Inspectability: решения видны через `SELECT * FROM chats ORDER BY relevance_score DESC`
- Гибкость: порог легко изменить без пересмотра всей логики
- Reason string объясняет решение

**Минусы**:
- Оценки захардкоджены — нет адаптации к конкретному пользователю
- Не учитывает activity ratio (пользователь мог молчать в группе 3 года)
- Бот с score 0.50 будет синхронизироваться — может быть spam bot

## Последствия

- `internal/application/sync/scorer.go` — единственное место с правилами
- `chats.relevance_score`, `chats.relevance_reason`, `chats.personality_surface` — всегда актуальны
- Изменение правил → изменить scorer.go → перезапустить backfill (scores перезапишутся)
- `filter.go` удалён, заменён `scorer.go`
