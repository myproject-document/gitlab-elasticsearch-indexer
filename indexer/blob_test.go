package indexer_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

func TestBuildBlob(t *testing.T) {
	file := gitFile("foo/bar", "foo")
	expected := validBlob(file, "foo", "Text")
	commitID := indexer.CommitID{ indexer.ProjectID(parentID), expected.CommitSHA }
	actualBlob, err := indexer.BuildBlob(file, commitID, "blob")

	require.NoError(t, err)
	require.Equal(t, expected, actualBlob)

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

	actualJSON, err := json.Marshal(actualBlob)
	require.NoError(t, err)
	require.JSONEq(t, expectedJSON, string(actualJSON))
}

func TestBuildBlobSkipsLargeBlobs(t *testing.T) {
	file := gitFile("foo/bar", "foo")
	file.Size = 1024*1024 + 1
	commitID := indexer.CommitID{ indexer.ProjectID(parentID), sha }
	blob, err := indexer.BuildBlob(file, commitID, "blob")

	require.Error(t, err, indexer.SkipTooLargeBlob)
	require.Nil(t, blob)
}

func TestBuildBlobSkipsBinaryBlobs(t *testing.T) {
	file := gitFile("foo/bar", "foo\x00")
	commitID := indexer.CommitID{ indexer.ProjectID(parentID), sha }
	blob, err := indexer.BuildBlob(file, commitID, "blob")

	require.Equal(t, err, indexer.SkipBinaryBlob)
	require.Nil(t, blob)
}

func TestBuildBlobDetectsLanguageByFilename(t *testing.T) {
	file := gitFile("Makefile.am", "foo")
	commitID := indexer.CommitID{ indexer.ProjectID(parentID), sha }
	blob, err := indexer.BuildBlob(file, commitID, "blob")

	require.NoError(t, err)
	require.Equal(t, "Makefile", blob.Language)
}

func TestBuildBlobDetectsLanguageByExtension(t *testing.T) {
	file := gitFile("foo.rb", "foo")
	commitID := indexer.CommitID{ indexer.ProjectID(parentID), sha }
	blob, err := indexer.BuildBlob(file, commitID, "blob")

	require.NoError(t, err)
	require.Equal(t, "Ruby", blob.Language)
}

func TestBlobIDRef(t *testing.T) {
	blobID := indexer.BlobID{ 2147483648, "path" }
	
	require.Equal(t, "2147483648_path", blobID.Ref())
}
