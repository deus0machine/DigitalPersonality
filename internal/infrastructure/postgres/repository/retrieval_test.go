package repository

import "testing"

func TestIsEmojiNoise(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"️", true},         // bare variation selector-16
		{"‍", true},         // bare zero-width joiner
		{"\U0001F3FB", true},     // bare skin tone modifier
		{"️‍", true},   // combination of combining marks
		{"❤", false},             // real emoji
		{"❤️", false},       // emoji with variation selector attached
		{"🤷‍♂", false},            // ZWJ sequence with base emoji
		{"👍", false},
	}
	for _, tt := range tests {
		if got := isEmojiNoise(tt.in); got != tt.want {
			t.Errorf("isEmojiNoise(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
