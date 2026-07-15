package utterance

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// msg builds a MessageInput with sane defaults for grouping tests.
func msg(id, authorID int64, offset time.Duration, text string) MessageInput {
	tokens := 0
	if text != "" {
		tokens = 1
	}
	return MessageInput{
		ID:             id,
		ChatID:         100,
		AuthorID:       authorID,
		SentAt:         t0.Add(offset),
		NormalizedText: text,
		TokenCount:     tokens,
	}
}

func TestBuildEmptyInput(t *testing.T) {
	if got := Build(nil, time.Minute); got != nil {
		t.Fatalf("Build(nil) = %v, want nil", got)
	}
}

func TestBuildGroupsSameAuthorWithinGap(t *testing.T) {
	msgs := []MessageInput{
		msg(1, 10, 0, "привет"),
		msg(2, 10, 30*time.Second, "как дела"),
		msg(3, 10, 60*time.Second, "что делаешь"),
	}
	got := Build(msgs, 2*time.Minute)

	if len(got) != 1 {
		t.Fatalf("got %d utterances, want 1", len(got))
	}
	u := got[0]
	if u.FirstMessageID != 1 {
		t.Errorf("FirstMessageID = %d, want 1 (first message of group)", u.FirstMessageID)
	}
	if u.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", u.MessageCount)
	}
	if u.Text != "привет как дела что делаешь" {
		t.Errorf("Text = %q", u.Text)
	}
	if !u.StartedAt.Equal(t0) || !u.EndedAt.Equal(t0.Add(60*time.Second)) {
		t.Errorf("time range = %v → %v", u.StartedAt, u.EndedAt)
	}
}

func TestBuildSplitsOnAuthorChange(t *testing.T) {
	msgs := []MessageInput{
		msg(1, 10, 0, "вопрос"),
		msg(2, 20, 10*time.Second, "ответ"),
		msg(3, 10, 20*time.Second, "реакция"),
	}
	got := Build(msgs, time.Minute)

	if len(got) != 3 {
		t.Fatalf("got %d utterances, want 3", len(got))
	}
	for i, wantID := range []int64{1, 2, 3} {
		if got[i].FirstMessageID != wantID {
			t.Errorf("utterance %d FirstMessageID = %d, want %d", i, got[i].FirstMessageID, wantID)
		}
		if got[i].Position != i {
			t.Errorf("utterance %d Position = %d, want %d", i, got[i].Position, i)
		}
	}
}

func TestBuildSplitsOnGap(t *testing.T) {
	gap := 2 * time.Minute
	msgs := []MessageInput{
		msg(1, 10, 0, "первое"),
		msg(2, 10, gap, "ровно на границе"), // <= gap merges
		msg(3, 10, 2*gap+time.Second, "после разрыва"),
	}
	got := Build(msgs, gap)

	if len(got) != 2 {
		t.Fatalf("got %d utterances, want 2 (gap boundary is inclusive)", len(got))
	}
	if got[0].MessageCount != 2 {
		t.Errorf("first utterance MessageCount = %d, want 2", got[0].MessageCount)
	}
	if got[1].FirstMessageID != 3 {
		t.Errorf("second utterance FirstMessageID = %d, want 3", got[1].FirstMessageID)
	}
}

func TestBuildDropsAllEmptyGroups(t *testing.T) {
	sticker := msg(1, 10, 0, "")
	sticker.MediaKind = "sticker"
	msgs := []MessageInput{
		sticker,
		msg(2, 20, 10*time.Second, "текст"),
	}
	got := Build(msgs, time.Minute)

	if len(got) != 1 {
		t.Fatalf("got %d utterances, want 1 (empty group dropped)", len(got))
	}
	if got[0].FirstMessageID != 2 {
		t.Errorf("FirstMessageID = %d, want 2", got[0].FirstMessageID)
	}
}

func TestBuildEmptyMessageInsideGroupCountedButNotJoined(t *testing.T) {
	sticker := msg(2, 10, 10*time.Second, "")
	sticker.MediaKind = "sticker"
	msgs := []MessageInput{
		msg(1, 10, 0, "смотри"),
		sticker,
		msg(3, 10, 20*time.Second, "смешно же"),
	}
	got := Build(msgs, time.Minute)

	if len(got) != 1 {
		t.Fatalf("got %d utterances, want 1", len(got))
	}
	if got[0].MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3 (empty message still counted)", got[0].MessageCount)
	}
	if got[0].Text != "смотри смешно же" {
		t.Errorf("Text = %q, want empty message excluded from join", got[0].Text)
	}
}

func TestBuildVoiceCounting(t *testing.T) {
	voice := msg(2, 10, 10*time.Second, "транскрипт голосового")
	voice.MediaKind = "voice"
	msgs := []MessageInput{
		msg(1, 10, 0, "щас запишу"),
		voice,
	}
	got := Build(msgs, time.Minute)

	if len(got) != 1 {
		t.Fatalf("got %d utterances, want 1", len(got))
	}
	if !got[0].HasVoice || got[0].VoiceCount != 1 {
		t.Errorf("HasVoice = %v, VoiceCount = %d; want true, 1", got[0].HasVoice, got[0].VoiceCount)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	msgs := []MessageInput{
		msg(1, 10, 0, "a"),
		msg(2, 10, 10*time.Second, "b"),
		msg(3, 20, 20*time.Second, "c"),
		msg(4, 10, 5*time.Minute, "d"),
	}
	first := Build(msgs, time.Minute)
	second := Build(msgs, time.Minute)

	if len(first) != len(second) {
		t.Fatalf("lengths differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].FirstMessageID != second[i].FirstMessageID {
			t.Errorf("run mismatch at %d: %d vs %d — FirstMessageID must be a stable embedding key",
				i, first[i].FirstMessageID, second[i].FirstMessageID)
		}
	}
}
