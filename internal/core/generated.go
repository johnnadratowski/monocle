package core

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// applyGenerated judges each changed file by the generated-file rule
// (types.GeneratedReason) from what the repo says: .gitattributes'
// linguist-generated, and the file's first lines in the working tree. It also
// reads what made the file from its header, and applies the tour's notes
// (types.MarkGenerated), which puts the generated files after the rest. A
// deleted file has no first lines, so only its path and attribute count; git
// failing (not a repo) leaves the attribute unknown.
func applyGenerated(repoRoot string, files []types.ChangedFile, tour *types.Walkthrough) []types.ChangedFile {
	if len(files) == 0 {
		return files
	}
	attrs := generatedAttrs(repoRoot, files)
	for i := range files {
		f := &files[i]
		head := ""
		if f.Status != types.FileDeleted && repoRoot != "" {
			head = fileHead(filepath.Join(repoRoot, f.Path))
		}
		f.Generated = types.GeneratedReason(f.Path, head, attrs[f.Path])
		f.GeneratedBy, f.GeneratedFrom = types.GeneratorOf(head)
	}
	return types.MarkGenerated(files, tour)
}

// generatedAttrs is `git check-attr linguist-generated` for the files, by path:
// "set", "true", "false", "unset" or "unspecified". Empty when git cannot say.
func generatedAttrs(repoRoot string, files []types.ChangedFile) map[string]string {
	if repoRoot == "" {
		return nil
	}
	args := []string{"check-attr", "-z", "linguist-generated", "--"}
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

// fileHead is a file's first types.GeneratedHeaderLines lines, each cut at 4 KB
// so a minified one-line file is not read whole; "" when it cannot be read.
func fileHead(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 4096)
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
