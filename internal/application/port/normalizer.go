// Package port defines output ports used by the application layer.
// Infrastructure packages provide implementations; application and domain
// packages reference only these clean contracts.
package port

import "github.com/digital-personality/internal/domain/entity"

// SemanticNormalizer converts a raw Message into a SemanticDocument
// suitable for embedding and semantic search.
//
// Contract:
//   - Never modifies the input message.
//   - Always returns a non-nil document (even for messages that should be skipped).
//   - SkipEmbedding = true signals that the message has no semantic value
//     for retrieval purposes (stickers, very short, emoji-only).
//   - Short or emoji-only messages MUST still produce a SemanticDocument
//     (with SkipEmbedding = true) so the pipeline knows normalization already ran.
type SemanticNormalizer interface {
	Normalize(msg *entity.Message) *entity.SemanticDocument
}
