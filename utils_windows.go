// +build windows

package ishell

import (
	"github.com/DavidLokison/readline"
)

func clearScreen(s *Shell) error {
	return readline.ClearScreen(s.writer)
}
