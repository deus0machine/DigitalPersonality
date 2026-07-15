package persona

import (
	"fmt"
	"strings"

	"github.com/digital-personality/internal/application/utterance"
)

// replySchema constrains generation to {"messages": ["...", ...]} so the
// persona always answers as a burst of separate Telegram-style messages.
const replySchema = `{
	"type": "object",
	"properties": {
		"messages": {
			"type": "array",
			"items": {"type": "string"},
			"minItems": 1,
			"maxItems": 6
		}
	},
	"required": ["messages"]
}`

// lengthClassLabels maps length classes to human-readable prompt descriptions.
var lengthClassLabels = []struct {
	class string
	label string
}{
	{"tiny", "крошечные (до 10 символов)"},
	{"short", "короткие (до 50 символов)"},
	{"medium", "средние (до 200 символов)"},
	{"long", "длинные (до 500 символов)"},
	{"very_long", "очень длинные"},
}

// BuildSystemPrompt renders the persona instruction from the style profile.
//
// Knowledge policy (deliberate design decision): knowledge leakage from the
// base LLM is allowed, but any leaked knowledge must be delivered strictly
// in the person's own voice — same message length, same slang, no lecturing.
func BuildSystemPrompt(profile *StyleProfile) string {
	var b strings.Builder

	b.WriteString("Ты — цифровая копия реального человека, восстановленная из его переписки в Telegram.\n")
	b.WriteString("Отвечай так, как ответил бы он: его словами, его длиной сообщений, его пунктуацией и сленгом.\n\n")

	b.WriteString("Его стиль по данным переписки:\n")
	if len(profile.LengthDist) > 0 {
		b.WriteString("- Длина сообщений: ")
		var parts []string
		for _, lc := range lengthClassLabels {
			if share, ok := profile.LengthDist[lc.class]; ok && share >= 0.01 {
				parts = append(parts, fmt.Sprintf("%.0f%% %s", share*100, lc.label))
			}
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString("\n")
	}
	if profile.AvgBurstSize > 0 {
		fmt.Fprintf(&b, "- Сообщений подряд за один раз: обычно %.0f, максимум примерно %.0f\n",
			profile.AvgBurstSize, profile.P90BurstSize)
	}
	if len(profile.TopSlang) > 0 {
		b.WriteString("- Частые слова и сленг: " + strings.Join(profile.TopSlang, ", ") + "\n")
	}
	if len(profile.TopEmoji) > 0 {
		b.WriteString("- Частые эмодзи: " + strings.Join(profile.TopEmoji, " ") + "\n")
	}

	b.WriteString(`
Правила:
1. Разбивай ответ на несколько коротких сообщений, как делает он. Каждое сообщение — отдельная строка массива.
2. Никаких лекций, списков, вежливых формул и тона ассистента.
3. Его сообщения из прошлых переписок — это ЕГО слова, используй их как образец стиля и источник фактов. Не выдумывай факты о своей жизни, которых там нет.
4. Реплики собеседника — ЧУЖИЕ слова. Никогда не выдавай их за свои, не повторяй вопрос собеседника и не отвечай его же словами.
5. Если темы нет в воспоминаниях — реагируй как он на незнакомое: коротко и честно, в его манере.
6. Если всё же делишься знанием, которого нет в воспоминаниях, — подавай его СТРОГО в его манере: те же короткие сообщения, тот же сленг, без умного лекторского тона.
7. Отвечай ПО СУЩЕСТВУ. Если просят рассказать что-то — расскажи конкретную вещь из воспоминаний (случай, факт, мнение), а не отделывайся пустыми словами.
8. Слова-заполнители («не суть», «понял», «ага», «ок») — максимум одно на весь ответ. Не начинай ответы одинаково и не повторяй свои прошлые реплики из диалога.
9. Если собеседник пишет бессмыслицу или набор букв — отреагируй на это по-своему (удивись, спроси), но НЕ копируй её.
10. Никогда не упоминай, что ты ИИ, копия или программа.

Ответ верни строго в JSON: {"messages": ["первое сообщение", "второе", ...]}
`)
	return b.String()
}

// BuildUserPrompt renders the person's own past messages (style + facts),
// the live dialog so far, and the incoming message.
// memories must be outgoing-only — the caller filters them; feeding
// interlocutors' lines here makes the model impersonate them.
func BuildUserPrompt(query string, memories []utterance.SearchResult, history []Turn) string {
	var b strings.Builder

	if len(memories) > 0 {
		b.WriteString("Его реальные сообщения из прошлых переписок — образцы стиля и фактов о нём:\n")
		for i, m := range memories {
			fmt.Fprintf(&b, "%d. [%s, чат «%s»]: %s\n",
				i+1,
				m.Utterance.StartedAt.Format("2006-01"),
				m.Utterance.ChatTitle,
				m.Utterance.Text,
			)
		}
		b.WriteString("\n")
	}

	if len(history) > 0 {
		b.WriteString("Текущий диалог:\n")
		for _, t := range history {
			role := "собеседник"
			if t.FromPersona {
				role = "он"
			}
			fmt.Fprintf(&b, "%s: %s\n", role, t.Text)
		}
		b.WriteString("\n")
	}

	b.WriteString("Собеседник пишет ему: " + query + "\n")
	b.WriteString("Ответь от его лица.")
	return b.String()
}
