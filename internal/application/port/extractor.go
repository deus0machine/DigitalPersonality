package port

import "github.com/digital-personality/internal/domain/entity"

// PersonalityExtractor derives personality signals from a single raw Message.
//
// Contract:
//   - Never modifies the input message.
//   - Returns zero signals (nil/empty slice) if the message has no extractable signal.
//   - Extraction is purely CPU-bound and must not perform I/O.
//   - Signal extraction is idempotent: re-running on the same message produces
//     the same signals (the repository layer handles the UPSERT).
//   - Does NOT generate personality profiles — that is a separate aggregation step.
//
// Why kept separate from SemanticNormalizer:
//   - Semantic normalization serves retrieval (embedding pipeline).
//   - Personality extraction serves simulation (personality engine).
//   - They can evolve independently and have different skip conditions.
//   - A sticker has rich personality signal but zero semantic content.
type PersonalityExtractor interface {
	Extract(msg *entity.Message) []entity.PersonalitySignal
}
