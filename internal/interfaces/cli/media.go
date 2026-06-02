package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/digital-personality/internal/application/retrieval"
)

// MediaInspect prints a comprehensive media audit report.
func (r *Runner) MediaInspect(ctx context.Context) error {
	rep, err := r.svc.MediaInspect(ctx)
	if err != nil {
		return fmt.Errorf("media inspect: %w", err)
	}

	printHeader("Media Inspect")

	// ── Global overview ───────────────────────────────────────────────────────
	fmt.Printf("  %-12s  %8s  %8s  %6s  %6s\n",
		"KIND", "TOTAL", "IN-WIN", "WIN%", "OUT%")
	printSeparator()
	for _, s := range rep.KindStats {
		winPct, outPct := 0, 0
		if s.Total > 0 {
			winPct = s.InWindow * 100 / s.Total
			outPct = s.Outgoing * 100 / s.Total
		}
		fmt.Printf("  %-12s  %8d  %8d  %5d%%  %5d%%\n",
			s.Kind, s.Total, s.InWindow, winPct, outPct)
	}
	printSeparator()

	// ── Voice ─────────────────────────────────────────────────────────────────
	printMediaKindDetail("Voice", rep.Voice)

	// ── Round ─────────────────────────────────────────────────────────────────
	printMediaKindDetail("Round (video messages)", rep.Round)

	// ── Sticker ───────────────────────────────────────────────────────────────
	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Sticker (%d total, %d in window)", rep.Sticker.Total, rep.Sticker.InWindow)))
	printMediaCounts(rep.Sticker.Total, rep.Sticker.InWindow, rep.Sticker.Outgoing)

	if len(rep.Sticker.TopEmoticons) > 0 {
		fmt.Println("\n  Top emoticons:")
		maxCnt := rep.Sticker.TopEmoticons[0].Count
		for _, e := range rep.Sticker.TopEmoticons {
			b := bar(e.Count, maxCnt, 16)
			fmt.Printf("    %s  %s  %d\n", e.Emoticon, b, e.Count)
		}
	} else {
		fmt.Println("\n  No emoticons stored (sticker_meta.Emoticon empty — expected for older syncs)")
	}

	if len(rep.Sticker.TopChats) > 0 {
		fmt.Println("\n  Top chats:")
		printMediaChatTable(rep.Sticker.TopChats)
	}
	if len(rep.Sticker.BySurface) > 0 {
		fmt.Println("\n  By surface:")
		printSurfaceTable(rep.Sticker.BySurface)
	}

	// ── Photo ─────────────────────────────────────────────────────────────────
	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Photo (%d total, %d in window)", rep.Photo.Total, rep.Photo.InWindow)))
	printMediaCounts(rep.Photo.Total, rep.Photo.InWindow, rep.Photo.Outgoing)
	if len(rep.Photo.TopChats) > 0 {
		fmt.Println("\n  Top chats:")
		printMediaChatTable(rep.Photo.TopChats)
	}
	if len(rep.Photo.BySurface) > 0 {
		fmt.Println("\n  By surface:")
		printSurfaceTable(rep.Photo.BySurface)
	}

	// ── Technical assessment ──────────────────────────────────────────────────
	printMediaAssessment(rep)

	return nil
}

func printMediaKindDetail(label string, d retrieval.MediaKindDetail) {
	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("%s (%d total, %d in window)", label, d.Total, d.InWindow)))
	printMediaCounts(d.Total, d.InWindow, d.Outgoing)

	if len(d.TopChats) > 0 {
		fmt.Println("\n  Top chats (all surfaces):")
		printMediaChatTable(d.TopChats)
	}
	if len(d.TopInterpersonal) > 0 {
		fmt.Println("\n  Top interpersonal chats:")
		printMediaChatTable(d.TopInterpersonal)
	}
	if len(d.TopSocial) > 0 {
		fmt.Println("\n  Top social chats:")
		printMediaChatTable(d.TopSocial)
	}
}

func printMediaCounts(total, inWindow, outgoing int) {
	incoming := total - outgoing
	winPct, outPct := 0, 0
	if total > 0 {
		winPct = inWindow * 100 / total
		outPct = outgoing * 100 / total
	}
	fmt.Printf("  Total:      %d\n", total)
	fmt.Printf("  In window:  %d  (%d%%)  ← processing candidates\n", inWindow, winPct)
	fmt.Printf("  Outgoing:   %d  (%d%%)\n", outgoing, outPct)
	fmt.Printf("  Incoming:   %d  (%d%%)\n", incoming, 100-outPct)
}

func printMediaChatTable(entries []retrieval.MediaChatEntry) {
	fmt.Printf("    %-34s  %-20s  %6s  %6s\n", "Chat", "Surface", "Total", "InWin")
	fmt.Println("   ", strings.Repeat("─", 64))
	for _, e := range entries {
		fmt.Printf("    %-34s  %-20s  %6d  %6d\n",
			truncate(e.Title, 34),
			truncate(formatSurface(e.Surface), 20),
			e.Total, e.InWindow,
		)
	}
}

func printSurfaceTable(entries []retrieval.MediaSurfaceEntry) {
	for _, e := range entries {
		winPct := 0
		if e.Total > 0 {
			winPct = e.InWindow * 100 / e.Total
		}
		fmt.Printf("    %-28s  %6d total  %6d in-win  (%d%%)\n",
			formatSurface(e.Surface), e.Total, e.InWindow, winPct)
	}
}

func printMediaAssessment(rep *retrieval.MediaInspectReport) {
	fmt.Printf("\n%s\n", sectionLine("Technical Assessment"))

	voiceWinPct := 0
	if rep.Voice.Total > 0 {
		voiceWinPct = rep.Voice.InWindow * 100 / rep.Voice.Total
	}
	roundWinPct := 0
	if rep.Round.Total > 0 {
		roundWinPct = rep.Round.InWindow * 100 / rep.Round.Total
	}

	fmt.Printf(`
  What's most valuable for digital personality:

  A. Personality signal value (high → low):
     1. Voice messages  — spoken language, tone, spontaneous expression
     2. Round video     — personal video notes, facial/emotional context
     3. Sticker usage   — emotional vocabulary, reaction patterns
     4. Photo           — shared content reveals interests (captions help)
     5. Video           — mostly consumed content, low authorship signal

  B. Information gain vs implementation cost:

     [HIGH ROI]  Voice transcription
                 %d in window (%d%% retention) → transcription candidates
                 Method: messages.transcribeAudio (Premium, no download)
                 Blocker: access_hash not stored — fix: 1 migration + 2 lines
                 Gain: converts %d silent messages → searchable semantic memory

     [ZERO COST] Sticker emoticon analysis
                 Already stored in sticker_meta JSONB — no API calls
                 %d stickers (%d in window)
                 Gain: emotional state signals from sticker choice patterns

     [MEDIUM ROI] Round transcription
                 %d in window (%d%% retention)
                 Same method as voice (transcribeAudio handles round too)
                 Add to transcription worker with zero extra infrastructure

     [PHASE 6+]  Photo / video analysis
                 Requires vision AI — not justified before embeddings are live

  C. Implementation priority:
     1. Voice + Round transcription  (after access_hash fix)
     2. Sticker emoticon aggregation (immediate — data already in DB)
     3. Photo captions               (text already stored, use directly)
     4. Vision AI for photos/video   (Phase 6+)
`,
		rep.Voice.InWindow, voiceWinPct, rep.Voice.InWindow,
		rep.Sticker.Total, rep.Sticker.InWindow,
		rep.Round.InWindow, roundWinPct,
	)
}

// VoiceStats prints a breakdown of voice messages across the database.
// Duration is not currently stored in the schema — noted in the output.
func (r *Runner) VoiceStats(ctx context.Context) error {
	stats, err := r.svc.VoiceStats(ctx)
	if err != nil {
		return fmt.Errorf("voice stats: %w", err)
	}

	printHeader("Voice Message Statistics")

	if stats.TotalVoice == 0 {
		fmt.Println("  No voice messages found.")
		fmt.Println("  (Voice messages are stored with media_kind='voice' during sync.)")
		return nil
	}

	incomingVoice := stats.TotalVoice - stats.OutgoingVoice
	outPct := 0
	winPct := 0
	if stats.TotalVoice > 0 {
		outPct = stats.OutgoingVoice * 100 / stats.TotalVoice
		winPct = stats.VoiceInWindow * 100 / stats.TotalVoice
	}

	fmt.Printf("  Total voice messages:      %d\n", stats.TotalVoice)
	fmt.Printf("  Outgoing (sent by user):   %d  (%d%%)\n", stats.OutgoingVoice, outPct)
	fmt.Printf("  Incoming (received):       %d  (%d%%)\n", incomingVoice, 100-outPct)
	fmt.Printf("  In memory window:          %d  (%d%%)  ← transcription candidates\n", stats.VoiceInWindow, winPct)
	fmt.Printf("  Duration:                  not stored (requires migration 000007)\n")

	// By surface.
	if len(stats.BySurface) > 0 {
		fmt.Println()
		fmt.Println("  Voice in memory window by surface:")
		for _, s := range stats.BySurface {
			fmt.Printf("    %-30s  %d\n", formatSurface(s.Surface), s.VoiceInWindow)
		}
	}

	if len(stats.TopChats) == 0 {
		return nil
	}

	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Top %d Chats by Voice Count", len(stats.TopChats))))
	fmt.Printf("  %-5s  %-34s  %-22s  %6s  %6s  %5s\n",
		"Score", "Chat", "Surface", "Voice", "InWin", "Out%")
	printSeparator()

	for _, e := range stats.TopChats {
		oPct := 0
		if e.VoiceCount > 0 {
			oPct = e.OutgoingCount * 100 / e.VoiceCount
		}
		fmt.Printf("  %.2f   %-34s  %-22s  %6d  %6d  %4d%%\n",
			e.Score,
			truncate(e.Title, 34),
			truncate(formatSurface(e.Surface), 22),
			e.VoiceCount,
			e.InWindowCount,
			oPct,
		)
	}

	printSeparator()
	fmt.Printf("\n  Transcription candidates: %d voice messages with in_memory_window=TRUE\n", stats.VoiceInWindow)
	fmt.Printf("  Method: messages.transcribeAudio (Telegram Premium — no download needed)\n")

	return nil
}
