// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ishell

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linesFunc returns a readLine func that yields the supplied lines, then io.EOF.
func linesFunc(lines ...string) func() (string, error) {
	i := 0
	return func() (string, error) {
		if i >= len(lines) {
			return "", io.EOF
		}
		l := lines[i]
		i++
		return l, nil
	}
}

// drain reads the reader to EOF and returns the bytes consumed.
func drain(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(b)
}

func TestUninterpretedReaderSingleLine(t *testing.T) {
	// Seed line is returned first; readLine then EOFs.
	r := newUninterpretedReader("select 1;", linesFunc(), func(bool) {}, nil)
	assert.Equal(t, "select 1;\n", drain(t, r))
	assert.Equal(t, "select 1;", r.Raw())
}

func TestUninterpretedReaderContinuation(t *testing.T) {
	// Seed plus a pulled continuation line.
	r := newUninterpretedReader("select 1", linesFunc(";"), func(bool) {}, nil)
	assert.Equal(t, "select 1\n;\n", drain(t, r))
	assert.Equal(t, "select 1\n;", r.Raw())
}

func TestUninterpretedReaderSeedServedAtPrimaryPrompt(t *testing.T) {
	// The first read returns the seed line without switching to the multi-line
	// prompt; the continuation prompt is only engaged when pulling later lines.
	var multi []bool
	r := newUninterpretedReader("select 1", linesFunc(";"), func(b bool) { multi = append(multi, b) }, nil)
	buf := make([]byte, 64)
	n, err := r.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "select 1\n", string(buf[:n]))
	assert.Empty(t, multi)
}

func TestUninterpretedReaderSpecialTerminatorStripped(t *testing.T) {
	// A trailing special terminator is stripped from the scanner feed and ends
	// the command; Raw keeps the original line.
	r := newUninterpretedReader(`select 1\G`, linesFunc("should not be read"), func(bool) {}, []string{`\g`, `\G`})
	assert.Equal(t, "select 1\n", drain(t, r))
	assert.Equal(t, `select 1\G`, r.Raw())
}

func TestUninterpretedReaderSpecialTerminatorWithTrailingSpace(t *testing.T) {
	r := newUninterpretedReader(`select 1 \G`, linesFunc(), func(bool) {}, []string{`\g`, `\G`})
	assert.Equal(t, "select 1 \n", drain(t, r))
	assert.Equal(t, `select 1 \G`, r.Raw())
}

func TestUninterpretedReaderSpecialTerminatorOnContinuation(t *testing.T) {
	// Special terminator can arrive on a continuation line.
	r := newUninterpretedReader("select 1", linesFunc(`\G`), func(bool) {}, []string{`\g`, `\G`})
	assert.Equal(t, "select 1\n\n", drain(t, r))
	assert.Equal(t, "select 1\n\\G", r.Raw())
}

func TestUninterpretedReaderEOFFromReadLine(t *testing.T) {
	// EOF (Ctrl-D) from readLine on a continuation surfaces after the seed.
	r := newUninterpretedReader("select 1", linesFunc(), func(bool) {}, nil)
	got := drain(t, r)
	assert.Equal(t, "select 1\n", got)
}

func TestUninterpretedReaderAbortedOnContinuationEOF(t *testing.T) {
	// Ctrl-D (EOF) at the continuation prompt, before any terminator completes
	// the command, marks the read aborted so the caller discards the partial.
	r := newUninterpretedReader("select 1", linesFunc(), func(bool) {}, nil)
	drain(t, r)
	assert.True(t, r.aborted)
}

func TestUninterpretedReaderNotAbortedOnSpecialTerminator(t *testing.T) {
	// A \g/\G terminator also ends the reader via EOF, but it is a completed
	// statement, not an abort, so aborted stays false and the caller runs it.
	r := newUninterpretedReader(`select 1\G`, linesFunc("should not be read"), func(bool) {}, []string{`\g`, `\G`})
	drain(t, r)
	assert.False(t, r.aborted)
}

func TestUninterpretedReaderErrorSurvivesSpecialTerminator(t *testing.T) {
	// A non-EOF read error on a line that also ends in a special terminator must
	// still surface; the deferred error takes precedence over the terminator EOF.
	boom := io.ErrUnexpectedEOF
	readLine := func() (string, error) { return `2\G`, boom }
	r := newUninterpretedReader("select 1", readLine, func(bool) {}, []string{`\g`, `\G`})
	_, err := io.ReadAll(r)
	assert.Equal(t, boom, err)
}
