package entity

import "time"

// SemanticDocument is the Layer 2 (Semantic) representation of a message.
//
// It is derived from Message.Text but is a separate, independently queryable
// record. Normalization decisions (strip emoji, lowercase, etc.) live here —
// they NEVER mutate the raw Message.
//
// A message may have SkipEmbedding = true (pure emoji, sticker, very short)
// meaning it won't be sent to the embedding model but still exists in this
// table so we can track that normalization already ran.
type SemanticDocument struct {
	MessageID      int64
	NormalizedText string     // text processed for semantic search/embedding
	Language       string     // 'ru' | 'en' | 'mixed' | '' (unknown)
	TokenCount     int        // whitespace-token word count (for cost estimation)
	SkipEmbedding  bool       // true = don't embed; personality-only message
	CreatedAt      time.Time
	TranscribedAt  *time.Time // nil = not yet attempted; non-nil = processed (voice transcription)
}
