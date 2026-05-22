# ADR-0004 — Retrieval без embeddings (FTS + trigram)

_Дата: 2026-05-22 | Статус: принято_

## Контекст

Phase 5 (embedding pipeline) ещё не реализована. OpenAI API key не обязателен.
Нужен работающий retrieval уже сейчас — для инспекции данных, для разработки Phase 6,
для валидации качества ingestion.

Требования к retrieval:
1. Должен работать без внешних API
2. Поддерживать фильтрацию по метаданным (chat, surface, direction, time)
3. Давать ranked результаты
4. Быть аддитивным — embeddings добавятся поверх, не заменят

## Решение

Два механизма поверх PostgreSQL:

**Full-text search** (`tsvector` + GIN):
- `messages.text_search`: `GENERATED ALWAYS AS (to_tsvector('simple', COALESCE(text, ''))) STORED`
- `episode_semantic.text_search`: аналогично
- Query: `websearch_to_tsquery('simple', $1)` — поддерживает `AND`, кавычки, `-NOT`
- Rank: `ts_rank_cd(..., 4)` — нормализация по длине документа

**Trigram similarity** (`pg_trgm` + GIN):
- `GIN (text gin_trgm_ops)` на messages.text
- `similarity(m.text, $1) >= threshold` — fuzzy matching
- Default threshold: 0.30

**Стратегия FTS → trigram fallback**:
- Если FTS даёт результаты → используем их (точнее, лучший rank)
- Если FTS пуст → пробуем trigram (полезно для коротких слов, опечаток)
- Если нет текста в Query → возвращаем по метаданным (хронологически)

`MatchType` ("fts" | "trigram" | "filter") в каждом результате — explainability.

## Альтернативы

**Только FTS**: не работает для коротких запросов, опечаток, транслитерации.

**Только trigram**: медленнее на больших объёмах, нет BM25-like ranking.

**BM25 через расширение** (pg_bm25/ParadeDB): лучший ranking, но внешняя зависимость.
Отложено до Phase 5+ если FTS окажется недостаточным.

**ElasticSearch / Typesense**: отдельный сервис. Нарушает принцип минимализма на текущей фазе.

## Tradeoffs

**Плюсы**:
- Ноль внешних зависимостей — только PostgreSQL
- `websearch_to_tsquery` понимает русский (с 'simple' dictionary — без стемминга, см. KI-012)
- Trigram работает для частичных совпадений и опечаток
- Аддитивен: `SearchByVector` можно добавить к Repository interface позже

**Минусы**:
- 'simple' dictionary: нет морфологии для русского
- Trigram плохо работает на текстах короче 3 символов
- Нет семантического поиска: "грустно" не найдёт "печально"
- FTS ranking хуже BM25 для длинных запросов

## Последствия

- `retrieval.Repository` interface спроектирован так, что `SearchByVector` добавляется без breaking change
- `retrieval.Query.SimilarityThreshold` — настраиваемый порог
- Phase 5: добавить `message_embeddings` таблицу + `SearchByVector` в Repository
- Существующие FTS/trigram методы остаются как fallback и для фильтрации без вектора
