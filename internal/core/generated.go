package core

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// applyGenerated judges each changed file by the generated-file rule
// (types.JudgeGenerated) from what the change under review cannot write: the
// BASE's .gitattributes (linguist-generated), whether the path is at the base,
// and the first lines now and at the base. Whoever writes the change cannot
// fold a file away by adding a header, a .gitattributes line or a .gen. name to
// a file the base has; a file the change adds counts by its own header or name,
// and is marked so (GeneratedOwn). It also reads what made the file from its
// header, and puts the generated files after the rest (types.SortGenerated).
// A tour's notes play no part.
//
// No base (git cannot say) reads every file as new: the change's own signals,
// each marked so. git failing to read the base's attributes leaves them
// unknown, never the working tree's.
func applyGenerated(repoRoot, baseRef string, files []types.ChangedFile) []types.ChangedFile {
	if len(files) == 0 {
		return files
	}
	attrs := generatedAttrs(repoRoot, baseRef, files)
	atBase := pathsAtBase(repoRoot, baseRef, files)
	for i := range files {
		f := &files[i]
		in := types.GeneratedInput{Path: f.Path, Attr: attrs[f.Path], New: !atBase[f.Path], Deleted: f.Status == types.FileDeleted}
		if !in.Deleted && repoRoot != "" {
			in.Head = fileHead(filepath.Join(repoRoot, f.Path))
		}
		// The base's first lines matter only to a header the base must have too.
		if !in.New && (in.Deleted || types.HeaderMarked(in.Head)) {
			in.BaseHead = baseHead(repoRoot, baseRef, f.Path)
		}
		f.Generated, f.GeneratedOwn = types.JudgeGenerated(in)
		f.GeneratedNew, f.GeneratedHead = in.New, ""
		if in.New {
			f.GeneratedHead = in.Head
		}
		f.GeneratedBy, f.GeneratedFrom = "", ""
		if f.Generated != "" {
			head := in.Head
			if in.Deleted {
				head = in.BaseHead
			}
			f.GeneratedBy, f.GeneratedFrom = types.GeneratorOf(head)
		}
	}
	return types.SortGenerated(files)
}

// generatedAttrs is linguist-generated as the base's .gitattributes say, by
// path: "set", "true", "false", "unset" or "unspecified". Empty when there is
// no base or git cannot say (git before 2.40 has no --source).
func generatedAttrs(repoRoot, baseRef string, files []types.ChangedFile) map[string]string {
	if repoRoot == "" || baseRef == "" {
		return nil
	}
	args := []string{"check-attr", "--source", baseRef, "-z", "linguist-generated", "--"}
	for _, f := range files {
		args = append(args, f.Path)
	}
	out, err := NewGitClient(repoRoot).run(args...)
	if err != nil {
		return nil
	}
	// -z: path NUL attribute NUL value NUL, for each path.
	fields := strings.Split(out, "\x00")
	attrs := make(map[string]string, len(files))
	for i := 0; i+2 < len(fields); i += 3 {
		attrs[fields[i]] = fields[i+2]
	}
	return attrs
}

// pathsAtBase is which of the files' paths the base has. Empty when there is
// no base or git cannot say: every file is then new.
func pathsAtBase(repoRoot, baseRef string, files []types.ChangedFile) map[string]bool {
	if repoRoot == "" || baseRef == "" {
		return nil
	}
	args := []string{"--literal-pathspecs", "ls-tree", "-r", "-z", "--name-only", baseRef, "--"}
	for _, f := range files {
		args = append(args, f.Path)
	}
	out, err := NewGitClient(repoRoot).run(args...)
	if err != nil {
		return nil
	}
	at := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			at[p] = true
		}
	}
	return at
}

// baseHead is the file's first lines at the base, read as fileHead reads the
// working tree's; "" when git cannot show it.
func baseHead(repoRoot, baseRef, path string) string {
	if repoRoot == "" || baseRef == "" {
		return ""
	}
	cmd := exec.Command("git", "cat-file", "blob", baseRef+":"+path)
	cmd.Dir = repoRoot
	for _, kv := range os.Environ() { // git uses cmd.Dir as the repo root (GitClient.run)
		if !strings.HasPrefix(kv, "GIT_DIR=") && !strings.HasPrefix(kv, "GIT_WORK_TREE=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return ""
	}
	if cmd.Start() != nil {
		return ""
	}
	head := readHead(out)
	_ = cmd.Process.Kill() // a large blob need not be read to its end
	_ = cmd.Wait()
	return head
}

// fileHead is a file's first types.GeneratedHeaderLines lines; "" when it
// cannot be read.
func fileHead(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	return readHead(f)
}

// readHead is the first types.GeneratedHeaderLines lines, each cut at 4 KB so
// a minified one-line file is not read whole.
func readHead(rd io.Reader) string {
	r := bufio.NewReaderSize(rd, 4096)
	var lines []string
	for len(lines) < types.GeneratedHeaderLines {
		line, isPrefix, err := r.ReadLine()
		if err != nil {
			break
		}
		lines = append(lines, string(line))
		for isPrefix && err == nil { // the rest of a long line
			_, isPrefix, err = r.ReadLine()
		}
	}
	return strings.Join(lines, "\n")
}
