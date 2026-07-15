package persona

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/digital-personality/internal/application/utterance"
)

type stubRetriever struct {
	results []utterance.SearchResult
	err     error
}

func (s stubRetriever) Retrieve(context.Context, string, int64, int) ([]utterance.SearchResult, error) {
	return s.results, s.err
}

type stubStyle struct {
	profile *StyleProfile
	err     error
}

func (s stubStyle) LoadStyleProfile(context.Context, int) (*StyleProfile, error) {
	return s.profile, s.err
}

type stubGenerator struct {
	output string
	err    error
	// captured request for prompt assertions
	lastReq GenerateRequest
}

func (s *stubGenerator) Generate(_ context.Context, req GenerateRequest) (string, error) {
	s.lastReq = req
	return s.output, s.err
}

func testProfile() *StyleProfile {
	return &StyleProfile{
		LengthDist:    map[string]float64{"tiny": 0.42, "short": 0.54},
		TopSlang:      []string{"ну", "ага", "пон"},
		TopEmoji:      []string{"❤", "🥰"},
		AvgBurstSize:  2,
		P90BurstSize:  4,
		GapP50Seconds: 8,
		GapP90Seconds: 40,
	}
}

func memory(text string, outgoing bool) utterance.SearchResult {
	return utterance.SearchResult{
		Utterance: utterance.Utterance{Text: text, IsOutgoing: outgoing, ChatTitle: "тест"},
	}
}

func TestReplyParsesBurstMessages(t *testing.T) {
	gen := &stubGenerator{output: `{"messages": ["ну", "я подумаю", "об этом)"]}`}
	svc := NewService(
		stubRetriever{results: []utterance.SearchResult{memory("я хз вообще", true)}},
		stubStyle{profile: testProfile()},
		gen,
		120,
	)

	reply, err := svc.Reply(context.Background(), "что думаешь про квантовую физику?")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ну", "я подумаю", "об этом)"}
	if len(reply.Messages) != len(want) {
		t.Fatalf("got %d messages, want %d", len(reply.Messages), len(want))
	}
	for i := range want {
		if reply.Messages[i] != want[i] {
			t.Errorf("message %d = %q, want %q", i, reply.Messages[i], want[i])
		}
	}
	if reply.GapP50Seconds != 8 || reply.GapP90Seconds != 40 {
		t.Errorf("pacing = %v/%v, want 8/40 from style profile", reply.GapP50Seconds, reply.GapP90Seconds)
	}
}

func TestReplyPromptCarriesStyleAndMemories(t *testing.T) {
	gen := &stubGenerator{output: `{"messages": ["ок"]}`}
	svc := NewService(
		stubRetriever{results: []utterance.SearchResult{
			memory("мои отношения построены на взаимоуважении", true),
			memory("Сереж, а ты меня любишь?", false),
		}},
		stubStyle{profile: testProfile()},
		gen,
		120,
	)

	if _, err := svc.Reply(context.Background(), "расскажи про отношения"); err != nil {
		t.Fatal(err)
	}

	sys := gen.lastReq.System
	for _, want := range []string{"ну, ага, пон", "СТРОГО в его манере", "42%"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	user := gen.lastReq.User
	if !strings.Contains(user, "мои отношения построены") {
		t.Error("user prompt must contain the person's own memories")
	}
	if strings.Contains(user, "Сереж, а ты меня любишь") {
		t.Error("incoming memories must be filtered out — they make the model impersonate interlocutors")
	}
	if !strings.Contains(user, "расскажи про отношения") {
		t.Error("user prompt must contain the incoming message")
	}
	if gen.lastReq.Format == nil {
		t.Error("generation must request structured JSON output")
	}
	if gen.lastReq.MaxTokens == 0 {
		t.Error("generation must cap output tokens for speed")
	}
}

func TestReplyWithHistoryRendersDialog(t *testing.T) {
	gen := &stubGenerator{output: `{"messages": ["ок"]}`}
	svc := NewService(stubRetriever{}, stubStyle{profile: testProfile()}, gen, 120)

	history := []Turn{
		{FromPersona: false, Text: "ну привет"},
		{FromPersona: true, Text: "прив"},
		{FromPersona: true, Text: "норм?"},
	}
	if _, err := svc.ReplyWithHistory(context.Background(), "что делаешь", history); err != nil {
		t.Fatal(err)
	}

	user := gen.lastReq.User
	if !strings.Contains(user, "Текущий диалог:") {
		t.Error("user prompt must include the dialog section when history is present")
	}
	if !strings.Contains(user, "собеседник: ну привет") || !strings.Contains(user, "он: прив") {
		t.Error("history turns must be labeled by speaker")
	}
	if !strings.Contains(user, "что делаешь") {
		t.Error("user prompt must contain the current message")
	}
}

func TestReplyLimitsMemories(t *testing.T) {
	var many []utterance.SearchResult
	for range 30 {
		many = append(many, memory("моё сообщение", true))
	}
	gen := &stubGenerator{output: `{"messages": ["ок"]}`}
	svc := NewService(stubRetriever{results: many}, stubStyle{profile: testProfile()}, gen, 120)

	if _, err := svc.Reply(context.Background(), "вопрос"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(gen.lastReq.User, "моё сообщение"); n != defaultMemoryLimit {
		t.Errorf("prompt contains %d memories, want %d", n, defaultMemoryLimit)
	}
}

func TestReplyFallsBackToPlainTextOnBrokenJSON(t *testing.T) {
	gen := &stubGenerator{output: "ну хз, я не знаю"}
	svc := NewService(
		stubRetriever{},
		stubStyle{profile: testProfile()},
		gen,
		120,
	)

	reply, err := svc.Reply(context.Background(), "вопрос")
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Messages) != 1 || reply.Messages[0] != "ну хз, я не знаю" {
		t.Errorf("fallback reply = %v, want raw output as single message", reply.Messages)
	}
}

func TestReplyCapsMessageCount(t *testing.T) {
	gen := &stubGenerator{output: `{"messages": ["1","2","3","4","5","6","7","8"]}`}
	svc := NewService(stubRetriever{}, stubStyle{profile: testProfile()}, gen, 120)

	reply, err := svc.Reply(context.Background(), "вопрос")
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Messages) != maxReplyMessages {
		t.Errorf("got %d messages, want cap %d", len(reply.Messages), maxReplyMessages)
	}
}

func TestReplyErrorPaths(t *testing.T) {
	boom := errors.New("boom")
	profile := testProfile()

	cases := []struct {
		name string
		svc  *Service
	}{
		{"retriever error", NewService(stubRetriever{err: boom}, stubStyle{profile: profile}, &stubGenerator{}, 120)},
		{"style error", NewService(stubRetriever{}, stubStyle{err: boom}, &stubGenerator{}, 120)},
		{"generator error", NewService(stubRetriever{}, stubStyle{profile: profile}, &stubGenerator{err: boom}, 120)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.svc.Reply(context.Background(), "вопрос"); !errors.Is(err, boom) {
				t.Errorf("error not propagated: %v", err)
			}
		})
	}

	svc := NewService(stubRetriever{}, stubStyle{profile: profile}, &stubGenerator{}, 120)
	if _, err := svc.Reply(context.Background(), "   "); err == nil {
		t.Error("empty query must be rejected")
	}
}
