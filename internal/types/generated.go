package types

import (
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// A generated file — a lockfile, a typed client, a swagger spec — can be most
// of a change's lines and none of its decisions. The reviewer is not going to
// read it line by line, but needs to know what it is. So Monocle tells a
// generated file from a hand-written one and folds its diff to a card saying
// what it is. It still counts as unreviewed until the reviewer marks it: the
// card makes that quick, and a file folded away must never drop out of the
// review (John, 2026-10-31).
//
// The rule is shared with stage (the walkthrough skill's
// design/generated-files.md, whose cases tables generated_test.go copies): the
// first test that applies decides, and each answer has a reason. What it reads
// must not come from the change under review, or whoever writes the change
// could fold a file away (JudgeGenerated).

// The reasons a file is generated. A ChangedFile's Generated holds one, or ""
// for a hand-written file.
const (
	GeneratedByAttr   = "linguist-generated" // .gitattributes says so
	GeneratedLockfile = "lockfile"           // a package manager's lockfile
	GeneratedName     = "name"               // a .gen. or .generated. part in its name
	GeneratedCodegen  = "codegen output"     // a swagger spec
	GeneratedHeader   = "header"             // a marker in its first lines
)

// GeneratedHeaderLines is how many of a file's first lines the header test
// reads: every generator here writes its marker in the first comment block.
const GeneratedHeaderLines = 5

var lockfiles = []string{
	"pnpm-lock.yaml", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "bun.lock", "bun.lockb", "go.sum",
	"Cargo.lock", "Gemfile.lock", "poetry.lock", "uv.lock", "Pipfile.lock", "composer.lock", "flake.lock",
	"Package.resolved", "devbox.lock",
}

var codegenOutputs = []string{"swagger.json", "swagger.yaml"}

var (
	generatedName   = regexp.MustCompile(`\.(?:gen|generated)\.`)
	generatedHeader = regexp.MustCompile(`@generated|DO NOT EDIT|(?i:auto-?generated)`)
)

// GeneratedReason is the rule itself: why the file at repo path p is generated, or "" when it
// is hand-written. head is the file's first lines (only GeneratedHeaderLines
// are read; "" for a deleted file); attr is `git check-attr linguist-generated`'s
// value for it ("set", "true", "false", "unset", "unspecified"; "" not known).
func GeneratedReason(p, head, attr string) string {
	switch attr {
	case "false", "unset":
		return "" // the repo says this one is written by hand
	case "set", "true":
		return GeneratedByAttr
	}
	base := path.Base(p)
	switch {
	case slices.Contains(lockfiles, base):
		return GeneratedLockfile
	case generatedName.MatchString(base):
		return GeneratedName
	case slices.Contains(codegenOutputs, base):
		return GeneratedCodegen
	}
	if HeaderMarked(head) {
		return GeneratedHeader
	}
	return ""
}

// GeneratedInput is what JudgeGenerated reads of a changed file, each from
// where the change under review cannot write it.
type GeneratedInput struct {
	Path string
	// New: the path is not at the base, so the change adds it (a rename's new
	// path too). Deleted: it is gone from the working tree.
	New, Deleted bool
	// Head is the file's first lines now ("" when deleted); BaseHead, at the
	// base ("" when new).
	Head, BaseHead string
	// Attr is linguist-generated as the BASE's .gitattributes say: a line the
	// change adds to .gitattributes is the change speaking for itself.
	Attr string
}

// JudgeGenerated says why a changed file is generated, or "" when it is
// hand-written, so that the change under review cannot take a file out of
// review by its own say-so:
//   - the attribute is the base's (GeneratedInput.Attr);
//   - a file at the base counts by a header only when the base's version has
//     the marker too (a deleted file, by the base's version alone);
//   - a new file counts by its own header or name, the only signals it has.
//     own is then true: the card says the change classified itself.
//
// Whatever it says, a generated file still counts as unreviewed until marked.
func JudgeGenerated(in GeneratedInput) (reason string, own bool) {
	if in.New {
		reason = GeneratedReason(in.Path, in.Head, in.Attr)
		return reason, reason != "" && reason != GeneratedByAttr
	}
	// The path is the base's, so the attribute and the path rules stand.
	if reason = GeneratedReason(in.Path, "", in.Attr); reason != "" || in.Attr == "false" || in.Attr == "unset" {
		return reason, false
	}
	if !HeaderMarked(in.BaseHead) || (!in.Deleted && !HeaderMarked(in.Head)) {
		return "", false
	}
	return GeneratedHeader, false
}

// HeaderMarked reports whether a generated marker is in the first
// GeneratedHeaderLines lines.
func HeaderMarked(head string) bool {
	for _, ln := range headLines(head) {
		if generatedHeader.MatchString(ln) {
			return true
		}
	}
	return false
}

func headLines(head string) []string {
	lines := strings.Split(head, "\n")
	return lines[:min(len(lines), GeneratedHeaderLines)]
}

// What Monocle reads out of a header or a lockfile reaches the terminal, and a
// change's file can carry escape sequences and control bytes. So every capture
// is printable text ([^\s\p{Cc}], or a class without \p{Cc}) and runs to a
// delimiter: a control byte inside makes no capture, rather than a cut one.
// The card strips control bytes again before it draws anything.
var (
	generatedBy = regexp.MustCompile(`(?i)generated by:?\s+([\w@./-]+?)[.,;:]?(?:\s+from\s+([^\s\p{Cc}]+?)[.,;:]?)?(?:\s|$)`)
	sourceLine  = regexp.MustCompile(`(?i)^[\s/#*;-]*source:\s*([^\s\p{Cc}]+)(?:\s|$)`)
)

// GeneratorOf reads what made a file, and from what, out of its header: "Code
// generated by sqlc" and "// source: query.sql" give ("sqlc", "query.sql").
// Either is "" when the first GeneratedHeaderLines lines do not say.
func GeneratorOf(head string) (generator, source string) {
	for _, ln := range headLines(head) {
		if m := generatedBy.FindStringSubmatch(ln); m != nil && generator == "" {
			generator = m[1]
			if source == "" {
				source = m[2]
			}
		}
		if m := sourceLine.FindStringSubmatch(ln); m != nil && source == "" {
			source = m[1]
		}
	}
	return generator, source
}

// IsGenerated reports whether the rule calls the file generated. A tour's note
// never does: it is only the card's text for a file the rule already calls
// generated.
func (f ChangedFile) IsGenerated() bool { return f.Generated != "" }

// SortGenerated returns the files hand-written first, then generated, each in
// the order they came: a generated file sorts after the files someone wrote.
func SortGenerated(files []ChangedFile) []ChangedFile {
	if files == nil {
		return nil
	}
	sort.SliceStable(files, func(i, j int) bool { return !files[i].IsGenerated() && files[j].IsGenerated() })
	return files
}

// PackageChange is one package a lockfile's diff adds (From ""), removes (To
// "") or moves between versions.
type PackageChange struct {
	Name string
	From string
	To   string
}

// A lockfile format: how a diff line names a package and its version, either
// on one line (inline) or as a key line followed by a version line.
type lockFormat struct {
	inline  func(line string) (name, version string, ok bool)
	key     *regexp.Regexp // group 1: the package name
	version *regexp.Regexp // group 1: its version
}

var (
	pnpmKey    = regexp.MustCompile(`^  ['"]?/?([^\s'"\p{Cc}][^'"\p{Cc}]*?)['"]?:\s*$`)
	goSumLine  = regexp.MustCompile(`^([^\s\p{Cc}]+) (v[^\s/\p{Cc}]+)(?:/go\.mod)? [^\s\p{Cc}]+$`)
	npmKey     = regexp.MustCompile(`^\s*"(?:[^"]*node_modules/)((?:@[^/"\p{Cc}]+/)?[^/"\p{Cc}]+)": \{\s*$`)
	npmVersion = regexp.MustCompile(`^\s*"version": "([^"\p{Cc}]+)"`)
	yarnKey    = regexp.MustCompile(`^"?((?:@[^@"/\s\p{Cc}]+/)?[^@"\s\p{Cc}]+)@[^:]*:\s*$`)
	yarnVer    = regexp.MustCompile(`^\s+version:? "?([^"\s\p{Cc}]+)"?\s*$`)
	cargoKey   = regexp.MustCompile(`^name = "([^"\p{Cc}]+)"`)
	cargoVer   = regexp.MustCompile(`^version = "([^"\p{Cc}]+)"`)
)

// pnpmPackage reads a pnpm packages/snapshots key: "  '@types/node@20.2.0':",
// "  zod@3.23.8(typescript@5.6.3):" (a peer suffix is the same package),
// "  /chalk@5.0.0:" (lockfile v6). An importer ("  ui/packages/api:") has no
// version, so it is not one.
func pnpmPackage(line string) (string, string, bool) {
	m := pnpmKey.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	key, _, _ := strings.Cut(m[1], "(")
	at := strings.LastIndex(key, "@")
	if at <= 0 {
		return "", "", false
	}
	return key[:at], key[at+1:], true
}

func goSumPackage(line string) (string, string, bool) {
	m := goSumLine.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var lockFormats = map[string]lockFormat{
	"pnpm-lock.yaml":      {inline: pnpmPackage},
	"go.sum":              {inline: goSumPackage},
	"package-lock.json":   {key: npmKey, version: npmVersion},
	"npm-shrinkwrap.json": {key: npmKey, version: npmVersion},
	"yarn.lock":           {key: yarnKey, version: yarnVer},
	"Cargo.lock":          {key: cargoKey, version: cargoVer},
}

// LockfileChanges reads a lockfile's diff as package changes, by name: a
// package whose versions the diff only adds is added, only removes is removed,
// and both is bumped. nil for a file Monocle cannot read this way (not a
// lockfile, or one of a format it does not know) and for a diff that changes
// no package's version.
func LockfileChanges(d *DiffResult) []PackageChange {
	if d == nil {
		return nil
	}
	f, ok := lockFormats[path.Base(d.Path)]
	if !ok {
		return nil
	}
	removed, added := map[string][]string{}, map[string][]string{}
	note := func(set map[string][]string, name, version string) {
		if name != "" && version != "" && !slices.Contains(set[name], version) {
			set[name] = append(set[name], version)
		}
	}
	for _, h := range d.Hunks {
		// The package a version line belongs to, on each side: a key line in
		// context names it for both.
		var oldKey, newKey string
		for _, ln := range h.Lines {
			if f.inline != nil {
				name, version, ok := f.inline(ln.Content)
				switch {
				case !ok:
				case ln.Kind == DiffLineRemoved:
					note(removed, name, version)
				case ln.Kind == DiffLineAdded:
					note(added, name, version)
				}
				continue
			}
			if m := f.key.FindStringSubmatch(ln.Content); m != nil {
				switch ln.Kind {
				case DiffLineRemoved:
					oldKey = m[1]
				case DiffLineAdded:
					newKey = m[1]
				default:
					oldKey, newKey = m[1], m[1]
				}
				continue
			}
			if m := f.version.FindStringSubmatch(ln.Content); m != nil {
				switch ln.Kind {
				case DiffLineRemoved:
					note(removed, oldKey, m[1])
				case DiffLineAdded:
					note(added, newKey, m[1])
				}
			}
		}
	}
	var out []PackageChange
	names := map[string]bool{}
	for n := range removed {
		names[n] = true
	}
	for n := range added {
		names[n] = true
	}
	for n := range names {
		gone, came := without(removed[n], added[n]), without(added[n], removed[n])
		if len(gone) == 0 && len(came) == 0 {
			continue // the same versions on both sides: moved, not changed
		}
		out = append(out, PackageChange{Name: n, From: strings.Join(gone, ", "), To: strings.Join(came, ", ")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// without is a's versions that b does not have, in a's order.
func without(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
