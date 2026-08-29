// Copyright 2020 Dolthub, Inc.
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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanStatements(t *testing.T) {
	type testcase struct {
		input      string
		statements []string
		lineNums   []int
	}

	// Some of these include malformed input (e.g. strings that aren't properly terminated)
	testcases := []testcase{
		{
			input: `insert into foo values (";;';'");`,
			statements: []string{
				`insert into foo values (";;';'")`,
			},
		},
		{
			input: `select ''';;'; select ";\;"`,
			statements: []string{
				`select ''';;'`,
				`select ";\;"`,
			},
		},
		{
			input: `select ''';;'; select ";\;`,
			statements: []string{
				`select ''';;'`,
				`select ";\;`,
			},
		},
		{
			input: `select ''';;'; select ";\;
;`,
			statements: []string{
				`select ''';;'`,
				`select ";\;
;`,
			},
		},
		{
			input: `select '\\'''; select '";";'; select 1`,
			statements: []string{
				`select '\\'''`,
				`select '";";'`,
				`select 1`,
			},
		},
		{
			input: `select '\\''; select '";";'; select 1`,
			statements: []string{
				`select '\\''; select '";"`,
				`'; select 1`,
			},
		},
		{
			input: `insert into foo values(''); select 1`,
			statements: []string{
				`insert into foo values('')`,
				`select 1`,
			},
		},
		{
			input: `insert into foo values('''); select 1`,
			statements: []string{
				`insert into foo values('''); select 1`,
			},
		},
		{
			input: `insert into foo values(''''); select 1`,
			statements: []string{
				`insert into foo values('''')`,
				`select 1`,
			},
		},
		{
			input: `insert into foo values(""); select 1`,
			statements: []string{
				`insert into foo values("")`,
				`select 1`,
			},
		},
		{
			input: `insert into foo values("""); select 1`,
			statements: []string{
				`insert into foo values("""); select 1`,
			},
		},
		{
			input: `insert into foo values(""""); select 1`,
			statements: []string{
				`insert into foo values("""")`,
				`select 1`,
			},
		},
		{
			input: `select '\''; select "hell\"o"`,
			statements: []string{
				`select '\''`,
				`select "hell\"o"`,
			},
		},
		{
			input: `select * from foo; select baz from foo;
select
a from b; select 1`,
			statements: []string{
				"select * from foo",
				"select baz from foo",
				"select\na from b",
				"select 1",
			},
			lineNums: []int{
				1, 1, 2, 3,
			},
		},
		{
			input: "create table dumb (`hell\\`o;` int primary key);",
			statements: []string{
				"create table dumb (`hell\\`o;` int primary key)",
			},
		},
		{
			input: "create table dumb (`hell``o;` int primary key); select \n" +
				"baz from foo;\n" +
				"\n" +
				"select\n" +
				"a from b; select 1\n\n",
			statements: []string{
				"create table dumb (`hell``o;` int primary key)",
				"select \nbaz from foo",
				"select\na from b",
				"select 1",
			},
			lineNums: []int{
				1, 1, 4, 5,
			},
		},
		{
			input: `insert into foo values ('a', "b;", 'c;;""
'); update foo set baz = bar,
qux = '"hello"""' where xyzzy = ";;';'";


create table foo (a int not null default ';',
primary key (a));`,
			statements: []string{
				`insert into foo values ('a', "b;", 'c;;""
')`,
				`update foo set baz = bar,
qux = '"hello"""' where xyzzy = ";;';'"`,
				`create table foo (a int not null default ';',
primary key (a))`,
			},
			lineNums: []int{
				1, 2, 6,
			},
		},
		{
			input: `DELIMITER |
insert into foo values (1,2,3)|`,
			statements: []string{
				"",
				"insert into foo values (1,2,3)",
			},
			lineNums: []int{1, 2},
		},
		{
			// https://github.com/dolthub/dolt/issues/10828
			input: `-- comment asdfasdf
		delimiter //
		select current_user() //`,
			statements: []string{
				"",
				"",
				"select current_user()",
			},
			lineNums: []int{1, 2, 3},
		},
		{
			// https://github.com/dolthub/dolt/issues/8495
			input: strings.Repeat(" ", 4096) + `insert into foo values (1,2,3)`,
			statements: []string{
				"insert into foo values (1,2,3)",
			},
			lineNums: []int{1, 2},
		},
		{
			input: "DELIMITER" + strings.Repeat(" ", 4096) + `|
insert into foo values (1,2,3)|`,
			statements: []string{
				"",
				"insert into foo values (1,2,3)",
			},
			lineNums: []int{1, 2},
		},
		{
			// https://github.com/dolthub/dolt/issues/10694
			input: `-- '
-- can have intermediate comments
CALL dolt_commit('-m', 'message', '--allow-empty');
CALL dolt_checkout('main');`,
			statements: []string{
				"", "",
				"CALL dolt_commit('-m', 'message', '--allow-empty')",
				"CALL dolt_checkout('main')",
			},
			lineNums: []int{1, 2, 3, 4},
		},
		{
			input: `/* block comment with lone quote '
*/
-- can have intermediate comments
CALL dolt_commit('-m', 'message', '--allow-empty');
CALL dolt_checkout('main');`,
			statements: []string{
				`/* block comment with lone quote '
*/
-- can have intermediate comments
CALL dolt_commit('-m', 'message', '--allow-empty')`,
				"CALL dolt_checkout('main')",
			},
			lineNums: []int{1, 5},
		},
		{
			input: `select * /* -- ignore line comment inside block comment */ from xy;
select x from xy; -- select y from xy;
select * /* ignore multi-line comment with ;
comment;
comment;
*/ from foo;
select '-- ignore line comment
in quote';`,
			statements: []string{
				"select * /* -- ignore line comment inside block comment */ from xy",
				"select x from xy",
				"",
				`select * /* ignore multi-line comment with ;
comment;
comment;
*/ from foo`,
				`select '-- ignore line comment
in quote'`,
			},
			lineNums: []int{1, 2, 2, 3, 7},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.input, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			scanner := NewStreamScanner(reader)
			var i int
			for scanner.Scan() {
				require.True(t, i < len(tt.statements))
				assert.Equal(t, tt.statements[i], strings.TrimSpace(scanner.Text()))
				if tt.lineNums != nil {
					assert.Equal(t, tt.lineNums[i], scanner.StatementStartLine())
				} else {
					assert.Equal(t, 1, scanner.StatementStartLine())
				}
				i++
			}

			require.NoError(t, scanner.Err())
		})
	}
}

// TestEndedAtEOFFlag covers the per-Scan termination distinction that the
// shell completion check relies on. A delimiter-terminated statement must
// leave EndedAtEOF false; an unterminated trailing statement must leave it
// true.
func TestEndedAtEOFFlag(t *testing.T) {
	t.Run("delimiter terminated", func(t *testing.T) {
		scanner := NewStreamScanner(strings.NewReader("select 1;"))
		require.True(t, scanner.Scan())
		assert.False(t, scanner.EndedAtEOF())
		assert.False(t, scanner.Scan())
	})
	t.Run("unterminated EOF", func(t *testing.T) {
		scanner := NewStreamScanner(strings.NewReader("select 1"))
		require.True(t, scanner.Scan())
		assert.True(t, scanner.EndedAtEOF())
		assert.False(t, scanner.Scan())
	})
	t.Run("flag resets across scans", func(t *testing.T) {
		// First statement terminates; second hits EOF unterminated.
		scanner := NewStreamScanner(strings.NewReader("select 1; select 2"))
		require.True(t, scanner.Scan())
		assert.False(t, scanner.EndedAtEOF())
		require.True(t, scanner.Scan())
		assert.True(t, scanner.EndedAtEOF())
	})
}

// TestHasBufferedToken verifies the accessor the interactive shell uses to
// decide a command is complete without triggering a blocking read.
func TestHasBufferedToken(t *testing.T) {
	scanner := NewStreamScanner(strings.NewReader("select 1; select 2;"))
	require.True(t, scanner.Scan())
	// "select 2;" is still buffered after the first statement.
	assert.True(t, scanner.HasBufferedToken())
	require.True(t, scanner.Scan())
	// Only trailing (no) content remains.
	assert.False(t, scanner.HasBufferedToken())
}

// TestDelimiterAccessor checks the current delimiter is reported, and that it
// follows a DELIMITER statement when in-stream detection is enabled.
func TestDelimiterAccessor(t *testing.T) {
	assert.Equal(t, "//", NewStreamScannerWithDelimiter(strings.NewReader(""), "//").Delimiter())

	scanner := NewStreamScanner(strings.NewReader("DELIMITER |\nselect 1|"))
	require.True(t, scanner.Scan()) // empty token acks DELIMITER
	assert.Equal(t, "|", scanner.Delimiter())
}

// TestIgnoreDelimiterStatements verifies that disabling in-stream DELIMITER
// detection makes the scanner treat "DELIMITER x" as ordinary statement text
// rather than a delimiter change.
func TestIgnoreDelimiterStatements(t *testing.T) {
	// With detection (default): the DELIMITER is processed and the delimiter
	// becomes "|", so "select 1|" is its own statement.
	on := NewStreamScanner(strings.NewReader("DELIMITER |\nselect 1|"))
	require.True(t, on.Scan())
	assert.Equal(t, "", strings.TrimSpace(on.Text())) // empty DELIMITER ack
	require.True(t, on.Scan())
	assert.Equal(t, "select 1", strings.TrimSpace(on.Text()))

	// With detection disabled: no DELIMITER processing; with the default ";"
	// delimiter and no ";" present, the whole input is one statement.
	off := NewStreamScanner(strings.NewReader("DELIMITER |\nselect 1|"))
	off.IgnoreDelimiterStatements()
	require.True(t, off.Scan())
	assert.Equal(t, "DELIMITER |\nselect 1|", off.Text())
	assert.Equal(t, ";", off.Delimiter())
}
