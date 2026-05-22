package telegram

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// consoleAuthenticator implements auth.UserAuthenticator for interactive CLI auth.
// Phone is read from config; code is prompted on stdout.
// 2FA password is read from config if set, otherwise prompted.
type consoleAuthenticator struct {
	phone    string
	password string // 2FA cloud password — may be empty for accounts without 2FA
}

func (a *consoleAuthenticator) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a *consoleAuthenticator) Code(_ context.Context, sentCode *tg.AuthSentCode) (string, error) {
	switch sentCode.Type.(type) {
	case *tg.AuthSentCodeTypeApp:
		fmt.Print("Enter code from Telegram app: ")
	case *tg.AuthSentCodeTypeSMS:
		fmt.Print("Enter SMS code: ")
	case *tg.AuthSentCodeTypeCall:
		fmt.Print("Enter code from phone call: ")
	default:
		fmt.Print("Enter Telegram code: ")
	}
	return readLine()
}

func (a *consoleAuthenticator) Password(_ context.Context) (string, error) {
	if a.password != "" {
		return a.password, nil
	}
	fmt.Print("Enter 2FA password: ")
	return readLine()
}

func (a *consoleAuthenticator) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	fmt.Printf("Accepting Terms of Service (id=%s)\n", tos.ID.Data)
	return nil
}

func (a *consoleAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	// We only support existing accounts; new account registration is not implemented.
	return auth.UserInfo{}, fmt.Errorf("sign-up not supported: authenticate with an existing account")
}

func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}
