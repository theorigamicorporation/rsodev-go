package guard

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokens(t *testing.T) {
	got := strings.Join(tokens("allThings_byName HTTPServer v2"), " ")
	if got != "all things by name httpserver v2" {
		t.Fatalf("tokens: %q", got)
	}
	if Refused("an ordinary sentence about queues and apps") != "" {
		t.Fatal("ordinary text was refused")
	}
}

// TestRepositoryIsClean reads every tracked-looking file of the repository
// and fails on a name that must not be published.
func TestRepositoryIsClean(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == ".git" || name == "bin" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if name == "go.sum" || name == "coverage.out" || strings.HasSuffix(name, ".test") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for number, line := range strings.Split(string(raw), "\n") {
			if found := Refused(line); found != "" {
				t.Errorf("%s:%d: %q must not be in this repository", path, number+1, found)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
