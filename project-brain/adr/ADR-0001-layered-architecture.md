# ADR-0001 — Layered Architecture (Clean Architecture)

_Дата: 2026-05-22 | Статус: принято_

## Контекст

Проект строит AI-систему на основе Telegram-данных. Изначально можно было написать простой скрипт,
но долгосрочные цели (embeddings, LLM simulation, relationship graph) требуют изменяемой архитектуры.
Telegram клиент (gotd/td) — внешняя библиотека с нестабильным API. PostgreSQL + pgvector — конкретная технология,
которая может быть заменена или расширена. LLM провайдер ещё не выбран окончательно.

## Решение

Четырёхслойная Clean Architecture:

```
domain/          — сущности и интерфейсы репозиториев, ноль зависимостей
application/     — use cases, порты (interfaces), бизнес-оркестрация
infrastructure/  — postgres, telegram, normalizer, personality
interfaces/      — delivery (HTTP, CLI) — пока не реализован
```

Зависимости строго внутрь. Инфраструктура реализует порты, определённые в application.

## Альтернативы

**Flat package структура**: быстрее писать, но Telegram-типы утекли бы везде.
Первый рефакторинг был бы катастрофой.

**Hexagonal (ports & adapters) без domain слоя**: проще, но domain entities
не имели бы чёткой принадлежности. Сложнее тестировать бизнес-логику изолированно.

**Microservices сразу**: premature. Один бинарник пока достаточно.

## Tradeoffs

**Плюсы**:
- Telegram-типы (tg.*) ограничены одним пакетом
- PostgreSQL-детали не видны из application слоя
- Каждый слой тестируется независимо
- LLM провайдер можно заменить без изменения domain

**Минусы**:
- Больше файлов, больше интерфейсов
- Маппинг на каждой границе (portToEntity, mapMessage и т.д.)
- Новому разработчику нужно объяснять структуру

## Последствия

- `internal/application/port/telegram.go` — единственное место где описаны Telegram-контракты
- Каждое изменение Telegram API требует изменения только в `infrastructure/telegram/`
- `internal/app/app.go` — explicit wiring, никакой магии dependency injection
