package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestConfirmationPromptAcceptsOnlyExplicitApproval(t *testing.T) {
	for _, answer := range []string{"y\n", "yes\n", "Y\n", "YES\n"} {
		prompt := NewConfirmationPrompt(strings.NewReader(answer), io.Discard, true)
		approved, err := prompt.Confirm(context.Background(), "delete tenant")
		if err != nil || !approved {
			t.Fatalf("answer %q: approved=%t err=%v", answer, approved, err)
		}
	}
}

func TestConfirmationPromptRefusesUnsafeInput(t *testing.T) {
	for _, answer := range []string{"n\n", "no\n", "\n", "maybe\n", ""} {
		prompt := NewConfirmationPrompt(strings.NewReader(answer), io.Discard, true)
		approved, err := prompt.Confirm(context.Background(), "delete tenant")
		if err != nil || approved {
			t.Fatalf("answer %q: approved=%t err=%v", answer, approved, err)
		}
	}
	prompt := NewConfirmationPrompt(strings.NewReader("y\n"), io.Discard, false)
	approved, err := prompt.Confirm(context.Background(), "delete tenant")
	if err != nil || approved {
		t.Fatalf("non-interactive prompt: approved=%t err=%v", approved, err)
	}
}

func TestConfirmationPromptDoesNotPrintWhenNoDecisionIsPossible(t *testing.T) {
	var output bytes.Buffer
	prompt := NewConfirmationPrompt(strings.NewReader(""), &output, true)
	approved, err := prompt.Confirm(context.Background(), "delete tenant")
	if err != nil || approved || output.Len() == 0 {
		t.Fatalf("approved=%t err=%v output=%q", approved, err, output.String())
	}
	prompt = NewConfirmationPrompt(strings.NewReader("y\n"), &output, false)
	output.Reset()
	_, _ = prompt.Confirm(context.Background(), "delete tenant")
	if output.Len() != 0 {
		t.Fatalf("non-interactive output = %q", output.String())
	}
}
