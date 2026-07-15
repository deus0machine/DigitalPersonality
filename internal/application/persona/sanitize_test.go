package persona

import (
	"context"
	"testing"
)

func TestSanitizeDropsQueryEcho(t *testing.T) {
	got := sanitizeMessages(
		[]string{"я саша, ты любишь сашу?", "ну а чё сразу так"},
		"я саша, ты любишь сашу?",
		nil,
	)
	if len(got) != 1 || got[0] != "ну а чё сразу так" {
		t.Errorf("got %v, want the echo dropped", got)
	}
}

func TestSanitizeDropsBurstDuplicates(t *testing.T) {
	got := sanitizeMessages(
		[]string{"не понял", "ага", "Не понял!", "ага 🥰"},
		"вопрос",
		nil,
	)
	if len(got) != 2 {
		t.Fatalf("got %v, want in-burst duplicates dropped", got)
	}
}

func TestSanitizeDropsRecentHistoryRepeats(t *testing.T) {
	history := []Turn{
		{FromPersona: false, Text: "как дела у тя, чмо"},
		{FromPersona: true, Text: "не понял"},
		{FromPersona: true, Text: "🥺"},
	}
	got := sanitizeMessages(
		[]string{"не поня", "🥺", "да нормально всё"},
		"где ты",
		history,
	)
	if len(got) != 1 || got[0] != "да нормально всё" {
		t.Errorf("got %v, want truncated repeat and emoji repeat dropped", got)
	}
}

func TestSanitizeKeepsFreshMessages(t *testing.T) {
	history := []Turn{
		{FromPersona: true, Text: "ну чё как?"},
	}
	msgs := []string{"вчера на пары ходил", "препод душный", "🥱"}
	got := sanitizeMessages(msgs, "как учеба", history)
	if len(got) != 3 {
		t.Errorf("got %v, want all fresh messages kept", got)
	}
}

func TestIsRepeatContainmentGuard(t *testing.T) {
	// Short quote of a much longer message is NOT a repeat.
	if isRepeat("рот", "че ты не понял, ебать твой рот") {
		t.Error("short quote of a long message must not count as a repeat")
	}
	// Truncated variant IS a repeat.
	if !isRepeat("не поня", "не понял") {
		t.Error("truncated self-repeat must be caught")
	}
}

func TestReplyRetriesOnFullRepetition(t *testing.T) {
	history := []Turn{
		{FromPersona: true, Text: "не понял"},
	}
	gen := &stubGenerator{outputs: []string{
		`{"messages": ["не понял", "не поня"]}`,  // first attempt: all repeats
		`{"messages": ["да живой я, чё хотел"]}`, // retry succeeds
	}}
	svc := NewService(stubRetriever{}, stubStyle{profile: testProfile()}, gen, 120)

	reply, err := svc.ReplyWithHistory(context.Background(), "ты тут?", history)
	if err != nil {
		t.Fatal(err)
	}
	if gen.calls != 2 {
		t.Errorf("generator called %d times, want 2 (retry after full repetition)", gen.calls)
	}
	if len(reply.Messages) != 1 || reply.Messages[0] != "да живой я, чё хотел" {
		t.Errorf("got %v, want the retry result", reply.Messages)
	}
}

func TestReplyDegradesWhenRetryAlsoRepeats(t *testing.T) {
	history := []Turn{
		{FromPersona: true, Text: "не понял"},
	}
	gen := &stubGenerator{outputs: []string{
		`{"messages": ["не понял"]}`,
		`{"messages": ["не понял", "не понял"]}`,
	}}
	svc := NewService(stubRetriever{}, stubStyle{profile: testProfile()}, gen, 120)

	reply, err := svc.ReplyWithHistory(context.Background(), "ты тут?", history)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Messages) != 1 {
		t.Errorf("got %v, want exactly one degraded message instead of silence", reply.Messages)
	}
}

func TestParseSplitsMultilineElements(t *testing.T) {
	got := parseReplyMessages(`{"messages": ["не поня\n🥰", "ок"]}`)
	if len(got) != 3 {
		t.Errorf("got %v, want newline-joined element split into separate messages", got)
	}
}
