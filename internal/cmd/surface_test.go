package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

const surfacePath = "testdata/surface.txt"

// updateSurfaceEnv names the variable that lets TestSurfaceSnapshot rewrite the
// snapshot, which it does only when SKILL.md's version carries the bump the
// change needs.
const updateSurfaceEnv = "YTR_UPDATE_SURFACE"

const restoreSurface = "restore it with git checkout HEAD -- internal/cmd/testdata/surface.txt and rerun"

var (
	skillVersionField = regexp.MustCompile(`(?m)^  version: "([^"]*)"$`)
	versionNumber     = regexp.MustCompile(`^(\d+)\.(\d+)$`)
)

// skillVersion is SKILL.md's metadata.version: an agent that learned the CLI
// from one version can tell from the next whether what it learned still runs.
type skillVersion struct {
	major, minor int
}

func (v skillVersion) String() string {
	return strconv.Itoa(v.major) + "." + strconv.Itoa(v.minor)
}

func (v skillVersion) less(w skillVersion) bool {
	return v.major < w.major || v.major == w.major && v.minor < w.minor
}

func parseSkillVersion(s string) (skillVersion, error) {
	match := versionNumber.FindStringSubmatch(s)
	if match == nil {
		return skillVersion{}, fmt.Errorf("version %q is not MAJOR.MINOR", s)
	}

	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])

	return skillVersion{major: major, minor: minor}, nil
}

func skillMetadataVersion(doc string) (skillVersion, error) {
	front, found := strings.CutPrefix(doc, "---\n")
	if found {
		front, _, found = strings.Cut(front, "\n---\n")
	}
	if !found {
		return skillVersion{}, errors.New("SKILL.md has no front matter")
	}

	match := skillVersionField.FindStringSubmatch(front)
	if match == nil {
		return skillVersion{}, errors.New("SKILL.md's front matter has no metadata.version")
	}

	return parseSkillVersion(match[1])
}

// surfaceLines returns what an agent can type, one sorted line each: every
// non-hidden command, every flag a command defines itself, with its shorthand
// and pflag type, and every --json field of a command. An inherited flag is
// listed once, under the command that defines it, the root's under ytr.
func surfaceLines(root *cobra.Command) []string {
	var lines []string
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}

		path := cmd.CommandPath()
		lines = append(lines, "command "+path)

		cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
			name := "--" + flag.Name
			if flag.Shorthand != "" {
				name += " -" + flag.Shorthand
			}
			lines = append(lines, fmt.Sprintf("flag %s %s %s", path, name, flag.Value.Type()))
		})

		fields, _ := runner.Fields(cmd)
		for _, field := range fields {
			lines = append(lines, fmt.Sprintf("field %s %s", path, field))
		}
	})
	slices.Sort(lines)

	return lines
}

func renderSurface(version skillVersion, lines []string) string {
	return "version " + version.String() + "\n" + strings.Join(lines, "\n") + "\n"
}

func parseSurface(snapshot string) (skillVersion, []string, error) {
	first, rest, _ := strings.Cut(snapshot, "\n")
	number, found := strings.CutPrefix(first, "version ")
	if !found {
		return skillVersion{}, nil, fmt.Errorf("first line %q is not `version MAJOR.MINOR`", first)
	}

	version, err := parseSkillVersion(number)
	if err != nil {
		return skillVersion{}, nil, err
	}

	return version, strings.Split(strings.TrimSuffix(rest, "\n"), "\n"), nil
}

func linesNotIn(lines, other []string) []string {
	var missing []string
	for _, line := range lines {
		if !slices.Contains(other, line) {
			missing = append(missing, line)
		}
	}

	return missing
}

// checkBump returns why to, SKILL.md's version, cannot describe the surface
// after when the snapshot recorded before at from. A removed line, a rename
// included, is an invocation that stops working, so it needs a new major; an
// added line needs a new minor; with neither, the version only must not go
// back.
func checkBump(before, after []string, from, to skillVersion) error {
	needed, why := from, "nothing changed since "+from.String()
	switch {
	case len(linesNotIn(before, after)) > 0:
		needed, why = skillVersion{major: from.major + 1}, "a line was removed since "+from.String()
	case len(linesNotIn(after, before)) > 0:
		needed, why = skillVersion{major: from.major, minor: from.minor + 1}, "a line was added since "+from.String()
	}

	if to.less(needed) {
		return fmt.Errorf("%s, so SKILL.md's metadata.version needs %s or later, and it says %s", why, needed, to)
	}

	return nil
}

func surfaceDiff(before, after []string) string {
	var diff strings.Builder
	for _, line := range linesNotIn(before, after) {
		diff.WriteString("  - " + line + "\n")
	}
	for _, line := range linesNotIn(after, before) {
		diff.WriteString("  + " + line + "\n")
	}

	return diff.String()
}

// surfaceUpdate decides what becomes of committed, the snapshot file or nil
// when there is none, given SKILL.md's version to and the tree's surface after.
// It returns the content to write, "" when committed already matches, or why
// committed may not stay: a bump too small for the change, refused even with
// update, or a change that fits but waits for update. A missing or unreadable
// snapshot is refused too, since without the last one there is no bump to check.
func surfaceUpdate(committed []byte, to skillVersion, after []string, update bool) (string, error) {
	want := renderSurface(to, after)
	if string(committed) == want {
		return "", nil
	}

	if committed == nil {
		return "", fmt.Errorf("%s is missing; %s", surfacePath, restoreSurface)
	}

	from, before, err := parseSurface(string(committed))
	if err != nil {
		return "", fmt.Errorf("%s: %w; %s", surfacePath, err, restoreSurface)
	}

	change := surfaceDiff(before, after)
	if change == "" {
		change = fmt.Sprintf("  version %s -> %s\n", from, to)
	}
	if err = checkBump(before, after, from, to); err != nil {
		return "", fmt.Errorf("%s does not match the tree's commands, flags and --json fields:\n%s%w. "+
			"Bump it in skills/ytr/SKILL.md, then run %s=1 go test ./internal/cmd -run TestSurfaceSnapshot",
			surfacePath, change, err, updateSurfaceEnv)
	}

	if !update {
		return "", fmt.Errorf(
			"%s does not match the tree's commands, flags and --json fields at SKILL.md version %s:\n%s"+
				"Run %s=1 go test ./internal/cmd -run TestSurfaceSnapshot and commit the file with the change",
			surfacePath,
			to,
			change,
			updateSurfaceEnv,
		)
	}

	return want, nil
}

// TestSurfaceSnapshot keeps testdata/surface.txt equal to the tree's surface at
// SKILL.md's version, so a commit that changes a command, flag or --json field
// also bumps the version agents read, by as much as the change breaks.
func TestSurfaceSnapshot(t *testing.T) {
	t.Parallel()

	to, err := skillMetadataVersion(readSkill(t))
	if err != nil {
		t.Fatal(err)
	}

	committed, err := os.ReadFile(surfacePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}

	update := os.Getenv(updateSurfaceEnv) == "1"
	rewrite, err := surfaceUpdate(committed, to, surfaceLines(newRootCmd(&output.Options{})), update)
	if err != nil {
		t.Fatal(err)
	}
	if rewrite == "" {
		return
	}

	if err = os.WriteFile(surfacePath, []byte(rewrite), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("rewrote %s at version %s", surfacePath, to)
}

func TestSurfaceUpdate(t *testing.T) {
	t.Parallel()

	v := func(major, minor int) skillVersion { return skillVersion{major: major, minor: minor} }
	before := []string{"command ytr issue list", "field ytr issue list key", "flag ytr issue list --limit int"}
	added := slices.Concat(before, []string{"flag ytr issue list --all bool"})
	removed := []string{"command ytr issue list", "flag ytr issue list --limit int"}

	const (
		addedTooLittle = "testdata/surface.txt does not match the tree's commands, flags and --json fields:\n" +
			"  + flag ytr issue list --all bool\n" +
			"a line was added since 11.1, so SKILL.md's metadata.version needs 11.2 or later, and it says 11.1. " +
			"Bump it in skills/ytr/SKILL.md, then run YTR_UPDATE_SURFACE=1 go test ./internal/cmd -run TestSurfaceSnapshot"
		removedTooLittle = "testdata/surface.txt does not match the tree's commands, flags and --json fields:\n" +
			"  - field ytr issue list key\n" +
			"a line was removed since 11.2, so SKILL.md's metadata.version needs 12.0 or later, and it says 11.3. " +
			"Bump it in skills/ytr/SKILL.md, then run YTR_UPDATE_SURFACE=1 go test ./internal/cmd -run TestSurfaceSnapshot"
		bumpedOnly = "testdata/surface.txt does not match the tree's commands, flags and --json fields " +
			"at SKILL.md version 11.2:\n  version 11.1 -> 11.2\n" +
			"Run YTR_UPDATE_SURFACE=1 go test ./internal/cmd -run TestSurfaceSnapshot and commit the file with the change"
		noSnapshot = "testdata/surface.txt is missing; " +
			"restore it with git checkout HEAD -- internal/cmd/testdata/surface.txt and rerun"
		malformed = "testdata/surface.txt: first line \"command ytr issue list\" is not `version MAJOR.MINOR`; " +
			"restore it with git checkout HEAD -- internal/cmd/testdata/surface.txt and rerun"
	)

	rows := []struct {
		name      string
		committed []byte
		to        skillVersion
		after     []string
		update    bool
		wantWrite string
		wantErr   string
	}{
		{
			name:      "exact match",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 1),
			after:     before,
		},
		{
			name:      "exact match with update",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 1),
			after:     before,
			update:    true,
		},
		{
			name:      "flag added at 11.1 without a bump",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 1),
			after:     added,
			wantErr:   addedTooLittle,
		},
		{
			name:      "flag added at 11.1 without a bump, update refused",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 1),
			after:     added,
			update:    true,
			wantErr:   addedTooLittle,
		},
		{
			name:      "flag added with a minor bump and update",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 2),
			after:     added,
			update:    true,
			wantWrite: renderSurface(v(11, 2), added),
		},
		{
			name:      "field removed at 11.2 with 11.3",
			committed: []byte(renderSurface(v(11, 2), before)),
			to:        v(11, 3),
			after:     removed,
			wantErr:   removedTooLittle,
		},
		{
			name:      "field removed at 11.2 with 11.3, update refused",
			committed: []byte(renderSurface(v(11, 2), before)),
			to:        v(11, 3),
			after:     removed,
			update:    true,
			wantErr:   removedTooLittle,
		},
		{
			name:      "unchanged with SKILL.md bumped",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 2),
			after:     before,
			wantErr:   bumpedOnly,
		},
		{
			name:      "unchanged with SKILL.md bumped and update",
			committed: []byte(renderSurface(v(11, 1), before)),
			to:        v(11, 2),
			after:     before,
			update:    true,
			wantWrite: renderSurface(v(11, 2), before),
		},
		{
			name:    "no snapshot",
			to:      v(11, 1),
			after:   before,
			wantErr: noSnapshot,
		},
		{
			name:    "no snapshot with update",
			to:      v(11, 1),
			after:   before,
			update:  true,
			wantErr: noSnapshot,
		},
		{
			name:      "malformed snapshot",
			committed: []byte(strings.Join(before, "\n") + "\n"),
			to:        v(11, 1),
			after:     before,
			wantErr:   malformed,
		},
		{
			name:      "malformed snapshot with update",
			committed: []byte(strings.Join(before, "\n") + "\n"),
			to:        v(11, 1),
			after:     before,
			update:    true,
			wantErr:   malformed,
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			write, err := surfaceUpdate(row.committed, row.to, row.after, row.update)

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if gotErr != row.wantErr {
				t.Errorf("error = %q,\nwant %q", gotErr, row.wantErr)
			}
			if write != row.wantWrite {
				t.Errorf("content to write = %q, want %q", write, row.wantWrite)
			}
		})
	}
}

func TestSurfaceBumpRule(t *testing.T) {
	t.Parallel()

	v := func(major, minor int) skillVersion { return skillVersion{major: major, minor: minor} }
	before := []string{"command ytr issue list", "flag ytr issue list --limit int"}
	added := slices.Concat(before, []string{"flag ytr issue list --all bool"})
	removed := before[:1]
	renamed := []string{"command ytr issue list", "flag ytr issue list --max int"}

	rows := []struct {
		name    string
		after   []string
		from    skillVersion
		to      skillVersion
		wantErr string
	}{
		{"added without a bump", added, v(11, 1), v(11, 1), "a line was added since 11.1, so " +
			"SKILL.md's metadata.version needs 11.2 or later, and it says 11.1"},
		{"added with a minor bump", added, v(11, 1), v(11, 2), ""},
		{"added with a major bump", added, v(11, 1), v(12, 0), ""},
		{"removed with a minor bump", removed, v(11, 2), v(11, 3), "a line was removed since 11.2, so " +
			"SKILL.md's metadata.version needs 12.0 or later, and it says 11.3"},
		{"removed with a major bump", removed, v(11, 2), v(12, 0), ""},
		{"renamed with a minor bump", renamed, v(11, 1), v(11, 2), "needs 12.0 or later"},
		{"renamed with a major bump", renamed, v(11, 1), v(12, 0), ""},
		{"unchanged", before, v(11, 1), v(11, 1), ""},
		{"unchanged with SKILL.md bumped", before, v(11, 1), v(11, 2), ""},
		{"unchanged with the version going back", before, v(11, 1), v(11, 0), "nothing changed since 11.1, so " +
			"SKILL.md's metadata.version needs 11.1 or later, and it says 11.0"},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			err := checkBump(before, row.after, row.from, row.to)

			switch {
			case row.wantErr == "" && err != nil:
				t.Errorf("checkBump(%s -> %s) = %v, want nil", row.from, row.to, err)
			case row.wantErr != "" && (err == nil || !strings.Contains(err.Error(), row.wantErr)):
				t.Errorf("checkBump(%s -> %s) = %v, want an error containing %q", row.from, row.to, err, row.wantErr)
			}
		})
	}
}
