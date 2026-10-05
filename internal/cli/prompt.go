package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

type ConfirmationPrompt struct {
	In          io.Reader
	Out         io.Writer
	Interactive bool
}

func NewConfirmationPrompt(in io.Reader, out io.Writer, interactive bool) ConfirmationPrompt {
	return ConfirmationPrompt{In: in, Out: out, Interactive: interactive}
}

func NewInteractiveConfirmationPrompt() ConfirmationPrompt {
	interactive := false
	if info, err := os.Stdin.Stat(); err == nil {
		interactive = info.Mode()&os.ModeCharDevice != 0
	}
	return NewConfirmationPrompt(os.Stdin, os.Stdout, interactive)
}

func (p ConfirmationPrompt) Confirm(ctx context.Context, message string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !p.Interactive || p.In == nil {
		return false, nil
	}
	if p.Out != nil {
		if _, err := fmt.Fprintf(p.Out, "%s [y/N]: ", message); err != nil {
			return false, err
		}
	}
	data, err := bufio.NewReader(p.In).ReadString('\n')
	if err != nil || len(data) == 0 {
		return false, nil
	}
	answer := strings.TrimSpace(string(data))
	return answer == "y" || answer == "yes" || answer == "Y" || answer == "YES", nil
}
