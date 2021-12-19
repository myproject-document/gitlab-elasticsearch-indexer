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

	i, _, _ := setupIndexer()
	actual := i.BuildCommit(gitCommit)

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

func TestGenerateCommitID(t *testing.T) {
	require.Equal(t, "2147483648_sha", indexer.GenerateCommitID(2147483648, "sha"))
}
