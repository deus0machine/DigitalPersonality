package cli

import (
	"context"
	"fmt"
)

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
	if stats.TotalVoice > 0 {
		outPct = stats.OutgoingVoice * 100 / stats.TotalVoice
	}

	fmt.Printf("  Total voice messages:      %d\n", stats.TotalVoice)
	fmt.Printf("  Outgoing (sent by user):   %d  (%d%%)\n", stats.OutgoingVoice, outPct)
	fmt.Printf("  Incoming (received):       %d  (%d%%)\n", incomingVoice, 100-outPct)
	fmt.Printf("  Duration:                  not stored (requires migration 000007)\n")

	if len(stats.TopChats) == 0 {
		return nil
	}

	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Top %d Chats by Voice Count", len(stats.TopChats))))
	fmt.Printf("  %-5s  %-34s  %-22s  %6s  %5s\n",
		"Score", "Chat", "Surface", "Voice", "Out%")
	printSeparator()

	for _, e := range stats.TopChats {
		oPct := 0
		if e.VoiceCount > 0 {
			oPct = e.OutgoingCount * 100 / e.VoiceCount
		}
		fmt.Printf("  %.2f   %-34s  %-22s  %6d  %4d%%\n",
			e.Score,
			truncate(e.Title, 34),
			truncate(formatSurface(e.Surface), 22),
			e.VoiceCount,
			oPct,
		)
	}

	printSeparator()
	fmt.Printf("\n  Transcription options:\n")
	fmt.Printf("    messages.transcribeAudio  — requires Telegram Premium (or trial quota ~3/week)\n")
	fmt.Printf("    Whisper (OpenAI)          — requires document metadata (not stored; needs migration)\n")

	return nil
}
