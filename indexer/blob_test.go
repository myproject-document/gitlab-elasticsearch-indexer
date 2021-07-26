package indexer_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

func TestBuildBlob(t *testing.T) {
	file := gitFile("foo/bar", "foo")
	expected := validBlob(file, "foo", "Text")

	actual, err := indexer.BuildBlob(file, parentID, expected.CommitSHA, "blob", setupEncoder())
	require.NoError(t, err)

	require.Equal(t, expected, actual)

	expectedJSON := `{
		"commit_sha" : "` + expected.CommitSHA + `",
		"content"    : "` + expected.Content + `",
		"file_name"  : "` + expected.Filename + `",
		"language"   : "` + expected.Language + `",
		"oid"        : "` + expected.OID + `",
		"path"       : "` + expected.Path + `",
		"rid"        : "` + expected.RepoID + `",
		"type"       : "blob"
	}`

	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, expectedJSON, string(actualJSON))
}

func TestBuildBlobSkipsIndexingContentForLargeBlobs(t *testing.T) {
	file := gitFile("foo/bar", "foo")
	file.SkipTooLarge = true
	expected := validBlob(file, "", "Text")

	actual, err := indexer.BuildBlob(file, parentID, sha, "blob", setupEncoder())
	require.NoError(t, err)
	require.Equal(t, expected, actual)

	expectedJSON := `{
		"commit_sha" : "` + expected.CommitSHA + `",
		"content"    : "` + expected.Content + `",
		"file_name"  : "` + expected.Filename + `",
		"language"   : "` + expected.Language + `",
		"oid"        : "` + expected.OID + `",
		"path"       : "` + expected.Path + `",
		"rid"        : "` + expected.RepoID + `",
		"type"       : "blob"
	}`

	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, expectedJSON, string(actualJSON))
}

func TestBuildBlobBinaryBlobs(t *testing.T) {
	file := gitFile("foo/bar", "foo\x00")

	blob, err := indexer.BuildBlob(file, parentID, sha, "blob", setupEncoder())
	require.NoError(t, err)
	require.Equal(t, blob.Content, indexer.NoCodeContentMsgHolder)
}

func TestBuildBlobDetectsLanguageByFilename(t *testing.T) {
	file := gitFile("Makefile.am", "foo")
	blob, err := indexer.BuildBlob(file, parentID, sha, "blob", setupEncoder())

	require.NoError(t, err)
	require.Equal(t, "Makefile", blob.Language)
}

func TestBuildBlobDetectsLanguageByExtension(t *testing.T) {
	file := gitFile("foo.rb", "foo")
	blob, err := indexer.BuildBlob(file, parentID, sha, "blob", setupEncoder())

	require.NoError(t, err)
	require.Equal(t, "Ruby", blob.Language)
}

func TestGenerateBlobID(t *testing.T) {
	require.Equal(t, "2147483648_path", indexer.GenerateBlobID(2147483648, "path"))

	large_filename := strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 20)
	require.Equal(t, "12345678_e0264f90b84a0fe08768dc5dcdf27efe60fe6633", indexer.GenerateBlobID(12345678, large_filename))
}
