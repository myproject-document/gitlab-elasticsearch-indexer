package indexer_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

func TestBuildCommit(t *testing.T) {
	gitCommit := gitCommit("Initial commit")

	expected := validCommit(gitCommit)
	actual := indexer.BuildCommit(gitCommit, indexer.ProjectID(parentID))

	require.Equal(t, expected, actual)

	expectedJSON := `{
		"sha"       : "` + expected.SHA + `",
		"message"   : "` + expected.Message + `",
		"author"    : {
			"name": "` + expected.Author.Name + `",
			"email": "` + expected.Author.Email + `",
			"time": "` + indexer.GenerateDate(gitCommit.Author.When) + `"
		},
		"committer" : {
			"name": "` + expected.Committer.Name + `",
			"email": "` + expected.Committer.Email + `",
			"time": "` + indexer.GenerateDate(gitCommit.Committer.When) + `"
		},
		"rid"       : "` + expected.RepoID + `",
		"type"      : "commit"
	}`

	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, expectedJSON, string(actualJSON))
}

func TestCommitIDRef(t *testing.T) {
	commitID := indexer.CommitID{ indexer.ProjectID(2147483648), "sha" }
	require.Equal(t, "2147483648_sha", commitID.Ref())
}
