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
	"strings"
)

// uninterpretedReader adapts a line-at-a-time line source into an io.Reader
// that a StreamScanner can consume one statement at a time. It hands back the
// already-read seed (first) line first, then pulls subsequent lines on demand
// via readLine, switching to the continuation prompt for those lines. If a line
// ends with one of |specials| (e.g. "\g", "\G"), that terminator is stripped
// from the bytes fed to the scanner and the reader reports io.EOF once drained,
// so the command completes at that line. Raw returns the original lines
// (terminators intact) joined by "\n".
type uninterpretedReader struct {
	readLine func() (string, error)
	setMulti func(bool)
	specials []string

	seed     string
	seedUsed bool
	pending  []byte
	rawLines []string
	done     bool  // a special terminator ended the command
	aborted  bool  // genuine EOF (e.g. Ctrl-D) reached before the command completed
	err      error // a deferred read error to surface after pending drains
}

func newUninterpretedReader(seed string, readLine func() (string, error), setMulti func(bool), specials []string) *uninterpretedReader {
	return &uninterpretedReader{readLine: readLine, setMulti: setMulti, specials: specials, seed: seed}
}

func (r *uninterpretedReader) nextLine() (string, error) {
	if !r.seedUsed {
		r.seedUsed = true
		return r.seed, nil
	}
	// every line after the first is a continuation line
	r.setMulti(true)
	return r.readLine()
}

func (r *uninterpretedReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		// Surface a deferred read error before a special-terminator EOF, so a
		// real error is not lost when a line both errors and ends in \g/\G.
		if r.err != nil {
			return 0, r.err
		}
		if r.done {
			return 0, io.EOF
		}
		line, err := r.nextLine()
		if err != nil {
			if err == io.EOF {
				// Genuine end of input (e.g. Ctrl-D) reached while pulling a
				// continuation line, before any delimiter or special terminator
				// completed the command. The caller discards the partial input.
				r.aborted = true
			}
			if line == "" {
				return 0, err
			}
			// surface the error after the line's bytes are delivered
			r.err = err
		}
		r.rawLines = append(r.rawLines, line)
		feed := line
		trimmed := strings.TrimRight(line, " \t")
		for _, sc := range r.specials {
			if strings.HasSuffix(trimmed, sc) {
				feed = strings.TrimSuffix(trimmed, sc)
				r.done = true
				break
			}
		}
		r.pending = append([]byte(feed), '\n')
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// Raw returns the original input lines (with any special terminators intact)
// joined by "\n", matching the buffer the line-accumulation path produced.
func (r *uninterpretedReader) Raw() string {
	return strings.Join(r.rawLines, "\n")
}
