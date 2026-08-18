package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func chdir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func commandEmitsFetchTick(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	return msgContainsFetchTick(cmd())
}

func msgContainsFetchTick(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case fetchTickMsg:
		return true
	case tea.BatchMsg:
		for _, cmd := range msg {
			if commandEmitsFetchTick(cmd) {
				return true
			}
		}
	}
	return false
}

func TestListFilesLabelsDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := ListFiles(dir)
	want := []string{"file.txt", "nested/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListFiles() = %#v, want %#v", got, want)
	}
}

func TestInitialModelOutsideGitRepoListsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# notes"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	m := initialModel(false, dir)

	if m.isGitRepo {
		t.Fatal("initialModel(false) marked directory as a git repo")
	}
	if m.dir != dir {
		t.Fatalf("dir = %q, want %q", m.dir, dir)
	}
	if !reflect.DeepEqual(m.files, []string{"notes.md"}) {
		t.Fatalf("files = %#v, want %#v", m.files, []string{"notes.md"})
	}
	if body := m.renderBody(); !strings.Contains(body, "Files:") || !strings.Contains(body, "notes.md") {
		t.Fatalf("renderBody() = %q, want file listing", body)
	}
}

func TestRefreshUpdatesNonGitFileListing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	m := initialModel(false, dir)
	if len(m.files) != 0 {
		t.Fatalf("initial files = %#v, want empty", m.files)
	}

	if err := os.WriteFile(filepath.Join(dir, "later.txt"), []byte("later"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	becameGit := m.refresh()

	if becameGit {
		t.Fatal("refresh reported git transition in a non-git directory")
	}
	if !reflect.DeepEqual(m.files, []string{"later.txt"}) {
		t.Fatalf("files = %#v, want %#v", m.files, []string{"later.txt"})
	}
}

func TestRefreshDetectsTransitionIntoGitRepo(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	m := initialModel(false, dir)
	git(t, dir, "init")

	becameGit := m.refresh()

	if !becameGit {
		t.Fatal("refresh did not report transition into a git repo")
	}
	if !m.isGitRepo {
		t.Fatal("refresh did not mark model as a git repo")
	}
	if m.repoName != filepath.Base(dir) {
		t.Fatalf("repoName = %q, want %q", m.repoName, filepath.Base(dir))
	}
	if m.files != nil {
		t.Fatalf("files = %#v, want nil after entering git repo", m.files)
	}
	if m.branch == "" {
		t.Fatal("branch was empty after entering git repo")
	}
}

func TestManualRefreshMarksCachedUpstreamStatusStale(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	git(t, dir, "init")

	m := model{
		isGitRepo:     true,
		dir:           dir,
		repoName:      filepath.Base(dir),
		branch:        "main",
		behind:        2,
		upstreamSeen:  true,
		upstreamStale: false,
		viewport:      viewport.New(80, 20),
		ready:         true,
	}

	m.refreshAndMarkUpstreamStale()

	if !m.upstreamStale {
		t.Fatal("refresh did not mark cached upstream status stale")
	}
	if view := m.View(); !strings.Contains(view, "2 behind") || !strings.Contains(view, "stale upstream status") {
		t.Fatalf("View() = %q, want stale upstream status feedback", view)
	}
}

func TestManualRefreshSchedulesUpstreamRefresh(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	git(t, dir, "init")

	m := model{
		isGitRepo:    true,
		dir:          dir,
		repoName:     filepath.Base(dir),
		branch:       "main",
		behind:       2,
		upstreamSeen: true,
		viewport:     viewport.New(80, 20),
		ready:        true,
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !commandEmitsFetchTick(cmd) {
		t.Fatal("manual refresh did not schedule an upstream ahead/behind refresh")
	}
}

func TestPeriodicTickDoesNotScheduleUpstreamRefreshForExistingGitRepo(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	git(t, dir, "init")

	m := model{
		isGitRepo:    true,
		dir:          dir,
		repoName:     filepath.Base(dir),
		branch:       "main",
		behind:       2,
		upstreamSeen: true,
		viewport:     viewport.New(80, 20),
		ready:        true,
	}

	_, cmd := m.Update(tickMsg{})

	if commandEmitsFetchTick(cmd) {
		t.Fatal("periodic tick scheduled an upstream ahead/behind refresh for an existing git repo")
	}
}

func TestRenderBodyShowsGitStatusReadFailure(t *testing.T) {
	m := model{
		isGitRepo: true,
		statusErr: errors.New("git status failed"),
	}

	body := m.renderBody()
	if !strings.Contains(body, "Unable to read git status") || !strings.Contains(body, "git status failed") {
		t.Fatalf("renderBody() = %q, want git status failure feedback", body)
	}
}

func TestInitialModelCapturesGitStatusFailure(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	fakeGit := filepath.Join(fakeBin, "git")
	script := `#!/bin/sh
if [ "$1" = "status" ]; then
  exit 128
fi
if [ "$1" = "branch" ]; then
  printf 'main\n'
  exit 0
fi
if [ "$1" = "rev-parse" ] && [ "$2" = "--show-toplevel" ]; then
  pwd
  exit 0
fi
if [ "$1" = "rev-parse" ] && [ "$2" = "--verify" ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	m := initialModel(true, dir)
	if m.statusErr == nil {
		t.Fatal("initialModel did not capture git status failure")
	}
	if body := m.renderBody(); !strings.Contains(body, "Unable to read git status") {
		t.Fatalf("renderBody() = %q, want git status failure feedback", body)
	}
}

func TestGetRepoName(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	git(t, dir, "init")

	if got, want := GetRepoName(), filepath.Base(dir); got != want {
		t.Fatalf("GetRepoName() = %q, want %q", got, want)
	}
}

func TestGetGitStatusWithErrorDoesNotSilentlyLookCleanWhenGitStatusFails(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	fakeGit := filepath.Join(fakeBin, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nexit 128\n"), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	changes, err := GetGitStatusWithError()
	if err == nil {
		t.Fatal("GetGitStatusWithError returned nil error when git status failed")
	}
	if changes != nil {
		t.Fatalf("changes = %#v, want nil on git status failure", changes)
	}
}
