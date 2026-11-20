package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// staticDirFixture creates a directory laid out like the frontend build
// output: index.html and assets/app.js.
func staticDirFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "dist")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadStaticDir_Unset(t *testing.T) {
	setBaseEnv(t)

	dir, err := LoadStaticDir()
	if err != nil || dir != "" {
		t.Fatalf("LoadStaticDir() = %q, %v; want \"\", nil", dir, err)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("Load() without STATIC_DIR: %v", err)
	}
}

func TestLoadStaticDir_Valid(t *testing.T) {
	setBaseEnv(t)
	want := staticDirFixture(t)
	t.Setenv("STATIC_DIR", want+string(filepath.Separator))

	dir, err := LoadStaticDir()
	if err != nil {
		t.Fatalf("LoadStaticDir(): %v", err)
	}
	if dir != want {
		t.Errorf("LoadStaticDir() = %q, want the cleaned path %q", dir, want)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("Load() with a valid STATIC_DIR: %v", err)
	}
}

func TestLoadStaticDir_Invalid(t *testing.T) {
	root := t.TempDir()

	noIndex := filepath.Join(root, "no-index")
	if err := os.MkdirAll(filepath.Join(noIndex, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	indexIsDir := filepath.Join(root, "index-is-dir")
	if err := os.MkdirAll(filepath.Join(indexIsDir, "index.html"), 0o755); err != nil {
		t.Fatal(err)
	}

	plainFile := filepath.Join(root, "file.txt")
	if err := os.WriteFile(plainFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	valid := staticDirFixture(t)

	cases := []struct {
		name    string
		value   string
		wantErr string
	}{
		{"empty", "", "set but empty"},
		{"blank", "   ", "set but empty"},
		{"surrounding whitespace", " " + valid, "whitespace"},
		{"missing directory", filepath.Join(root, "does-not-exist"), "STATIC_DIR"},
		{"not a directory", plainFile, "not a directory"},
		{"without index.html", noIndex, "index.html"},
		{"index.html is a directory", indexIsDir, "index.html"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("STATIC_DIR", tc.value)

			dir, err := LoadStaticDir()
			if err == nil {
				t.Fatalf("LoadStaticDir() = %q, nil; want an error", dir)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("LoadStaticDir() error = %q, want it to contain %q", err, tc.wantErr)
			}

			// Load reports the same error, so a wrong STATIC_DIR stops startup.
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "STATIC_DIR") {
				t.Errorf("Load() error = %v, want a STATIC_DIR error", err)
			}
		})
	}
}
