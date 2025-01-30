package modulefiles

import (
	"context"
	"path"
	"testing"

	"github.com/psanford/memfs"
	"github.com/stretchr/testify/assert"
)

type embedTest struct {
	fs        []string
	directive string
	expected  []string
}

func (tt embedTest) run(t *testing.T) {
	t.Parallel()
	t.Helper()

	fs := memfs.New()
	for _, f := range tt.fs {
		if !assert.NoError(t, fs.MkdirAll(path.Dir(f), 0777)) {
			return
		}
		if !assert.NoError(t, fs.WriteFile(f, []byte("content"), 0700)) {
			return
		}
	}

	var actual []string

	err := expandEmbed(context.Background(), fs, tt.directive, func(f string) {
		actual = append(actual, f)
	})

	if assert.NoError(t, err) {
		assert.ElementsMatch(t, actual, tt.expected)
	}

}

func TestExpandEmbed(t *testing.T) {
	t.Parallel()

	t.Run("simple match", embedTest{
		fs:        []string{"example.txt"},
		directive: "example.txt",
		expected:  []string{"example.txt"},
	}.run)

	t.Run("unitary-glob", embedTest{
		fs:        []string{"example.txt"},
		directive: "*.txt",
		expected:  []string{"example.txt"},
	}.run)

	t.Run("multi-glob", embedTest{
		fs:        []string{"foo.txt", "bar.txt"},
		directive: "*.txt",
		expected:  []string{"foo.txt", "bar.txt"},
	}.run)

	t.Run("no-match-glob", embedTest{
		fs:        []string{"foo.txt", "bar.txt"},
		directive: "fizz*",
		expected:  []string{},
	}.run)

	t.Run("no-match", embedTest{
		fs:        []string{"foo.txt", "bar.txt"},
		directive: "fizz.txt",
		expected:  []string{},
	}.run)

	t.Run("dir", embedTest{
		fs:        []string{"d/foo", "d/bar", "foo"},
		directive: "d",
		expected:  []string{"d/foo", "d/bar"},
	}.run)

	t.Run("dir-glob-all", embedTest{
		fs:        []string{"d/foo", "d/bar", "foo"},
		directive: "d/*",
		expected:  []string{"d/foo", "d/bar"},
	}.run)

	t.Run("dir-nested-single", embedTest{
		fs:        []string{"d/foo", "d/bar", "foo"},
		directive: "d/foo",
		expected:  []string{"d/foo"},
	}.run)

	t.Run("all", embedTest{
		fs:        []string{"d/foo", "d/bar", "foo"},
		directive: "*",
		expected:  []string{"d/foo", "d/bar", "foo"},
	}.run)

	t.Run("exclude-special", embedTest{
		fs:        []string{"d/_foo", "d/bar", "_foo", ".ignored", "d/.ignored"},
		directive: "*",
		expected:  []string{"d/bar"},
	}.run)
}
