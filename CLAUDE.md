# Digital Personality — Engineering Rules

## Project Goal

This project is a long-term AI infrastructure system for building a digital personality and memory engine based on Telegram data.

The system must:

* synchronize Telegram messages through MTProto,
* store and index memories,
* generate embeddings,
* provide semantic memory retrieval,
* simulate user communication style using LLMs.

The architecture must prioritize:

* scalability,
* maintainability,
* observability,
* extensibility,
* clean boundaries,
* production-readiness.

---

# Core Architecture Principles

## Clean Architecture

The project MUST follow Clean Architecture principles.

Layers:

* domain
* application/usecases
* infrastructure
* interfaces/delivery

Dependencies must point inward only.

Infrastructure must never leak into domain logic.

---

# Dependency Rules

## Forbidden

* No global mutable state.
* No singleton business services.
* No circular dependencies.
* No direct DB access outside repositories.
* No business logic inside handlers/controllers.
* No God Objects.
* No package-level hidden state.
* No tight coupling between modules.

---

# Project Structure

Expected structure:

/cmd
/internal
/domain
/application
/infrastructure
/interfaces
/migrations
/configs

Each package must have a single responsibility.

---

# Coding Standards

## General

* Prefer composition over inheritance.
* Prefer explicitness over magic.
* Small focused interfaces only.
* Context must be propagated everywhere.
* All external operations must support cancellation.
* Avoid premature abstractions.
* Avoid overengineering.
* Write readable code first.

---

# Error Handling

* Errors must be wrapped with context.
* No ignored errors.
* No panic in business logic.
* Structured logging required for all critical errors.

Use:

* errors.Join
* fmt.Errorf("...: %w", err)

---

# Logging

Use structured logging only.

Every important operation must include:

* operation name
* entity identifiers
* execution duration
* error context

Sensitive data must never be logged.

---

# Database Rules

* PostgreSQL is the source of truth.
* Use migrations only.
* No automatic schema sync.
* Repositories own DB access.
* Transactions must be explicit.
* pgvector will be used for embeddings.

---

# Telegram Integration Rules

Telegram integration must:

* support reconnects,
* support resumable sync,
* avoid duplicate ingestion,
* support incremental updates,
* preserve original metadata.

MTProto session data must be stored securely.

---

# Embedding Pipeline Rules

Embedding generation must:

* support batching,
* support retries,
* support queue processing,
* avoid duplicate embeddings,
* support future model replacement.

Embeddings are infrastructure, not business logic.

---

# AI/LLM Rules

LLMs are stateless generators.

Personality must emerge from:

* memory,
* retrieval,
* contextual prompting,
* communication history.

Never hardcode personality into prompts only.

---

# Performance Goals

The system must be designed for:

* hundreds of thousands of messages,
* long-term memory growth,
* semantic retrieval,
* background processing.

Avoid loading large datasets into memory.

Streaming and pagination are preferred.

---

# Security Rules

Never expose:

* Telegram sessions,
* API keys,
* private messages,
* embeddings data.

Secrets must come from environment variables only.

---

# Docker Rules

The project must:

* run fully through docker-compose,
* support local development,
* support Linux deployment,
* avoid environment-specific hacks.

---

# Testing Philosophy

Prefer:

* integration tests for repositories,
* unit tests for business logic,
* minimal mocking.

Avoid testing implementation details.

---

# Future Direction

The project will later include:

* long-term memory,
* relationship graphs,
* emotional modeling,
* autonomous memory retrieval,
* AI persona simulation,
* multimodal memory.

Architecture decisions should preserve future extensibility.
