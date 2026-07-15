package cli

import (
	"context"
	"fmt"
	"strconv"
)

const botLogDialogLimit = 200

// BotLog shows the bot conversation log.
// Without args: per-interlocutor summaries, most recent first.
// With a user-id: that person's full dialog with the persona, chronological.
func (r *Runner) BotLog(ctx context.Context, args []string) error {
	if len(args) > 0 {
		userID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid user-id %q: %w", args[0], err)
		}
		return r.printBotDialog(ctx, userID)
	}

	dialogs, err := r.botMsgRepo.ListDialogs(ctx)
	if err != nil {
		return fmt.Errorf("bot-log: %w", err)
	}

	printHeader("Bot Conversations")
	if len(dialogs) == 0 {
		fmt.Println("  No conversations recorded yet.")
		return nil
	}

	fmt.Printf("  %-12s  %-20s  %6s  %s\n", "USER ID", "USERNAME", "MSGS", "LAST ACTIVITY")
	printSeparator()
	for _, d := range dialogs {
		name := d.Username
		if name == "" {
			name = "(unknown)"
		}
		fmt.Printf("  %-12d  %-20s  %6d  %s\n",
			d.UserID, truncate(name, 20), d.Messages, d.LastAt.Local().Format("2006-01-02 15:04"))
	}
	printSeparator()
	fmt.Printf("\n  %d dialog(s). Use 'bot-log <user-id>' to read one.\n\n", len(dialogs))
	return nil
}

func (r *Runner) printBotDialog(ctx context.Context, userID int64) error {
	msgs, err := r.botMsgRepo.ListByUser(ctx, userID, botLogDialogLimit)
	if err != nil {
		return fmt.Errorf("bot-log: %w", err)
	}

	printHeader(fmt.Sprintf("Bot Dialog — user %d", userID))
	if len(msgs) == 0 {
		fmt.Println("  No messages from this user.")
		return nil
	}

	name := "(unknown)"
	for _, m := range msgs {
		if m.Username != "" {
			name = m.Username
			break
		}
	}
	fmt.Printf("  Interlocutor: %s   Messages: %d (showing up to %d)\n",
		name, len(msgs), botLogDialogLimit)
	printSeparator()

	for _, m := range msgs {
		who := name
		if m.FromPersona {
			who = "persona"
		}
		fmt.Printf("  [%s] %-12s  %s\n",
			m.CreatedAt.Local().Format("01-02 15:04"), truncate(who, 12), m.Text)
	}
	fmt.Println()
	return nil
}
