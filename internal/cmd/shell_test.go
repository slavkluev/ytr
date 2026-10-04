package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// shellReader parses a line as far as a POSIX shell must to tell which commands
// it runs and which words each receives. Nothing is expanded: a $(…) stays in
// its word as written, and the commands inside it count as commands of the line.
type shellReader struct {
	line     []rune
	pos      int
	commands [][]string
}

// simpleCommand gathers the words of one simple command as the reader meets them.
type simpleCommand struct {
	words  []string
	word   strings.Builder
	inWord bool

	// quoted is set once any part of the word was quoted, escaped or
	// substituted, which keeps a word of digits from naming a file descriptor.
	quoted bool

	// redirected is set from a redirection operator until the word it
	// redirects to, which the command never receives.
	redirected bool
}

// shellCommands returns the words of every simple command on line, the
// commands of each $(…) included, without redirections or the comment. It
// fails where a shell fails to run the line: on an unquoted ( or ), an
// unquoted glob character, an unterminated quote or $(…), and a redirection
// with no word to redirect to.
func shellCommands(line string) ([][]string, error) {
	r := &shellReader{line: []rune(line)}
	if err := r.readCommands(false); err != nil {
		return nil, err
	}

	return r.commands, nil
}

// readCommands reads up to the end of the line or, nested in a $(…), up to the
// ) that closes it.
func (r *shellReader) readCommands(nested bool) error {
	cmd := &simpleCommand{}
	for r.pos < len(r.line) {
		ch := r.line[r.pos]
		r.pos++

		var err error
		switch {
		case ch == ')' && nested:
			return r.finish(cmd)
		case ch == '(' || ch == ')':
			return fmt.Errorf("unquoted %q at column %d", ch, r.pos)
		case ch == '#' && !cmd.inWord:
			r.pos = len(r.line)
		case strings.ContainsRune(" \t\n\r", ch):
			cmd.endWord()
		case ch == '|' || ch == ';' || ch == '&':
			err = r.finish(cmd)
			cmd = &simpleCommand{}
		case ch == '<' || ch == '>':
			err = r.redirect(cmd)
		default:
			err = r.readWordPart(cmd, ch)
		}
		if err != nil {
			return err
		}
	}

	if nested {
		return errors.New("$( has no closing )")
	}

	return r.finish(cmd)
}

func (r *shellReader) finish(cmd *simpleCommand) error {
	cmd.endWord()
	if cmd.redirected {
		return fmt.Errorf("redirection before column %d has no target", r.pos)
	}
	if len(cmd.words) > 0 {
		r.commands = append(r.commands, cmd.words)
	}

	return nil
}

// redirect reads the rest of a redirection operator, such as >> or >&, and
// drops the file descriptor written right before it, as 2 in 2>&1.
func (r *shellReader) redirect(cmd *simpleCommand) error {
	if cmd.redirected && !cmd.inWord {
		return fmt.Errorf("redirection at column %d has no target", r.pos)
	}

	for r.pos < len(r.line) && strings.ContainsRune("<>&|", r.line[r.pos]) {
		r.pos++
	}

	if cmd.inWord && !cmd.quoted && strings.Trim(cmd.word.String(), "0123456789") == "" {
		cmd.word.Reset()
		cmd.inWord = false
	}
	cmd.endWord()
	cmd.redirected = true

	return nil
}

func (r *shellReader) readWordPart(cmd *simpleCommand, ch rune) error {
	switch {
	case ch == '\'':
		end := slices.Index(r.line[r.pos:], '\'')
		if end < 0 {
			return fmt.Errorf("quote at column %d is never closed", r.pos)
		}
		cmd.add(string(r.line[r.pos:r.pos+end]), true)
		r.pos += end + 1
	case ch == '"':
		return r.readDoubleQuoted(cmd)
	case ch == '\\':
		if r.pos < len(r.line) {
			cmd.add(string(r.line[r.pos]), true)
			r.pos++
		}
	case ch == '$' && r.pos < len(r.line) && r.line[r.pos] == '(':
		return r.readSubstitution(cmd)
	case strings.ContainsRune("*?[", ch):
		// zsh, the macOS default shell, aborts a command whose glob matches no
		// file, and none of these words names one.
		return fmt.Errorf("unquoted %q at column %d", ch, r.pos)
	default:
		cmd.add(string(ch), false)
	}

	return nil
}

func (r *shellReader) readDoubleQuoted(cmd *simpleCommand) error {
	opened := r.pos
	cmd.add("", true)

	for r.pos < len(r.line) {
		ch := r.line[r.pos]
		r.pos++

		switch {
		case ch == '"':
			return nil
		case ch == '\\' && r.pos < len(r.line) && strings.ContainsRune("\"\\$`", r.line[r.pos]):
			cmd.add(string(r.line[r.pos]), true)
			r.pos++
		case ch == '$' && r.pos < len(r.line) && r.line[r.pos] == '(':
			if err := r.readSubstitution(cmd); err != nil {
				return err
			}
		default:
			cmd.add(string(ch), true)
		}
	}

	return fmt.Errorf("quote at column %d is never closed", opened)
}

// readSubstitution reads the commands of a $(…) whose $ the reader has just
// passed, and leaves the substitution in cmd's word as written.
func (r *shellReader) readSubstitution(cmd *simpleCommand) error {
	r.pos++
	start := r.pos
	if err := r.readCommands(true); err != nil {
		return err
	}
	cmd.add("$("+string(r.line[start:r.pos]), true)

	return nil
}

func (c *simpleCommand) add(part string, quoted bool) {
	c.word.WriteString(part)
	c.inWord = true
	c.quoted = c.quoted || quoted
}

func (c *simpleCommand) endWord() {
	if !c.inWord {
		return
	}

	if c.redirected {
		c.redirected = false
	} else {
		c.words = append(c.words, c.word.String())
	}
	c.word.Reset()
	c.inWord, c.quoted = false, false
}

func TestShellCommands(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name    string
		line    string
		want    [][]string
		wantErr string
	}{
		{
			name: "words",
			line: "  ytr issue list --filter queue=PROJ\n",
			want: [][]string{{"ytr", "issue", "list", "--filter", "queue=PROJ"}},
		},
		{
			name: "quotes keep spaces and operators",
			line: `ytr issue list --query 'Queue: PROJ "Sort By": Updated' --jq ".items[] | .key"`,
			want: [][]string{
				{"ytr", "issue", "list", "--query", `Queue: PROJ "Sort By": Updated`, "--jq", ".items[] | .key"},
			},
		},
		{
			name: "escapes",
			line: `ytr comment create K-1 --body "say \"hi\" \n" me\(\)`,
			want: [][]string{{"ytr", "comment", "create", "K-1", "--body", `say "hi" \n`, "me()"}},
		},
		{
			name: "only space, tab and newline separate words",
			line: "ytr x a\u00a0b\tc\r\n",
			want: [][]string{{"ytr", "x", "a\u00a0b", "c"}},
		},
		{
			name: "quoted and escaped glob characters",
			line: `ytr issue list --jq '.items[].key' --query "a*b?" \*`,
			want: [][]string{{"ytr", "issue", "list", "--jq", ".items[].key", "--query", "a*b?", "*"}},
		},
		{
			name: "empty quoted word",
			line: `ytr issue view "" ''`,
			want: [][]string{{"ytr", "issue", "view", "", ""}},
		},
		{
			name: "pipe",
			line: "echo X | ytr bulk update --field p=c",
			want: [][]string{{"echo", "X"}, {"ytr", "bulk", "update", "--field", "p=c"}},
		},
		{
			name: "separators",
			line: "a;b && c || d & e",
			want: [][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}},
		},
		{
			name: "substitution in double quotes",
			line: `ytr user get "$(ytr comment list K-1 --json authorId --jq '.[0].authorId')"`,
			want: [][]string{
				{"ytr", "comment", "list", "K-1", "--json", "authorId", "--jq", ".[0].authorId"},
				{"ytr", "user", "get", "$(ytr comment list K-1 --json authorId --jq '.[0].authorId')"},
			},
		},
		{
			name: "substitution in a redirection target",
			line: "ytr completion bash > $(brew --prefix)/etc/bash_completion.d/ytr",
			want: [][]string{{"brew", "--prefix"}, {"ytr", "completion", "bash"}},
		},
		{
			name: "redirections",
			line: `ytr version 2>/dev/null >>log <in >& "out file" 2>&1 | cat`,
			want: [][]string{{"ytr", "version"}, {"cat"}},
		},
		{
			name: "quoted digits are a word, not a file descriptor",
			line: "ytr x '2'>f 3x>g",
			want: [][]string{{"ytr", "x", "2", "3x"}},
		},
		{
			name: "comment",
			line: "ytr version # a#b ( is not read",
			want: [][]string{{"ytr", "version"}},
		},
		{
			name: "hash inside a word",
			line: "ytr x a#b",
			want: [][]string{{"ytr", "x", "a#b"}},
		},
		{
			name: "comment line",
			line: "  # Filter by queue",
		},
		{name: "unquoted (", line: "ytr issue list --filter assignee=me()", wantErr: `unquoted '(' at column 36`},
		{name: "unquoted )", line: "ytr x )", wantErr: `unquoted ')' at column 7`},
		{name: "unquoted [", line: "ytr issue list --jq .items[].key", wantErr: `unquoted '[' at column 27`},
		{name: "unquoted *", line: "ytr x a*", wantErr: `unquoted '*' at column 8`},
		{name: "unquoted ?", line: "ytr x ?", wantErr: `unquoted '?' at column 7`},
		{name: "process substitution", line: "source <(ytr completion bash)", wantErr: `unquoted '('`},
		{name: "open single quote", line: "ytr x 'abc", wantErr: "quote at column 7 is never closed"},
		{name: "open double quote", line: `ytr x "abc`, wantErr: "quote at column 7 is never closed"},
		{name: "open substitution", line: "ytr user get $(ytr user myself", wantErr: "$( has no closing )"},
		{name: "redirection without target", line: "ytr version >", wantErr: "has no target"},
		{name: "redirection into a pipe", line: "ytr version > | cat", wantErr: "has no target"},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			got, err := shellCommands(row.line)

			if row.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), row.wantErr) {
					t.Fatalf("shellCommands(%q) error = %v, want it to contain %q", row.line, err, row.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("shellCommands(%q): %v", row.line, err)
			}
			if !slices.EqualFunc(got, row.want, slices.Equal) {
				t.Errorf("shellCommands(%q) = %q, want %q", row.line, got, row.want)
			}
		})
	}
}
