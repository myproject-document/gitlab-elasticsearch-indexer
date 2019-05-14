package main_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pb "gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	gitalyClient "gitlab.com/gitlab-org/gitaly/client"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

var (
	binary         = flag.String("binary", "./bin/gitlab-elasticsearch-indexer", "Path to `gitlab-elasticsearch-indexer` binary for integration tests")
	gitalyConnInfo *gitalyConnectionInfo
)

const (
	projectID         = "667"
	headSHA           = "b83d6e391c22777fca1ed3012fce84f633d7fed0"
	testRepo          = "test-gitlab-elasticsearch-indexer/gitlab-test.git"
	testRepoPath      = "https://gitlab.com/gitlab-org/gitlab-test.git"
	testRepoNamespace = "test-gitlab-elasticsearch-indexer"
)

type gitalyConnectionInfo struct {
	Address string `json:"address"`
	Storage string `json:"storage"`
}

func init() {
	gci, exists := os.LookupEnv("GITALY_CONNECTION_INFO")
	if exists {
		json.Unmarshal([]byte(gci), &gitalyConnInfo)
	}
}

func ensureGitalyRepository(t *testing.T) {
	conn, err := gitalyClient.Dial(gitalyConnInfo.Address, gitalyClient.DefaultDialOpts)
	require.NoError(t, err)

	namespace := pb.NewNamespaceServiceClient(conn)
	repository := pb.NewRepositoryServiceClient(conn)

	// Remove the repository if it already exists, for consistency
	rmNsReq := &pb.RemoveNamespaceRequest{StorageName: gitalyConnInfo.Storage, Name: testRepoNamespace}
	_, err = namespace.RemoveNamespace(context.Background(), rmNsReq)
	require.NoError(t, err)

	gl_repository := &pb.Repository{StorageName: gitalyConnInfo.Storage, RelativePath: testRepo}
	createReq := &pb.CreateRepositoryFromURLRequest{Repository: gl_repository, Url: testRepoPath}

	_, err = repository.CreateRepositoryFromURL(context.Background(), createReq)
	require.NoError(t, err)
}

func checkDeps(t *testing.T) {
	if os.Getenv("ELASTIC_CONNECTION_INFO") == "" {
		t.Skip("ELASTIC_CONNECTION_INFO not set")
	}

	if os.Getenv("GITALY_CONNECTION_INFO") == "" {
		t.Skip("GITALY_CONNECTION_INFO is not set")
	}

	if testing.Short() {
		t.Skip("Test run with -short, skipping integration test")
	}

	if _, err := os.Stat(*binary); err != nil {
		t.Skip("No binary found at ", *binary)
	}
}

func buildIndex(t *testing.T) (*elastic.Client, func()) {
	railsEnv := fmt.Sprintf("test-integration-%d", time.Now().Unix())
	os.Setenv("RAILS_ENV", railsEnv)

	client, err := elastic.FromEnv(projectID)
	require.NoError(t, err)

	require.NoError(t, client.CreateIndex())

	return client, func() {
		client.DeleteIndex()
	}
}

func run(from, to string) error {
	cmd := exec.Command(*binary, projectID, testRepo)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// GitLab always sets FROM_SHA
	if from == "" {
		from = "0000000000000000000000000000000000000000"
	}

	cmd.Env = append(cmd.Env, "FROM_SHA="+from)

	if to != "" {
		cmd.Env = append(cmd.Env, "TO_SHA="+to)
	}

	return cmd.Run()
}

func TestIndexingRemovesFiles(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildIndex(t)
	defer td()

	// The commit before files/empty is removed - so it should be indexed
	require.NoError(t, run("", "19e2e9b4ef76b422ce1154af39a91323ccc57434"))
	_, err := c.GetBlob("files/empty")
	require.NoError(t, err)

	// Now we expect it to have been removed
	require.NoError(t, run("19e2e9b4ef76b422ce1154af39a91323ccc57434", "08f22f255f082689c0d7d39d19205085311542bc"))
	_, err = c.GetBlob("files/empty")
	require.Error(t, err)
}

type document struct {
	Blob      *indexer.Blob     `json:"blob"`
	Commit    *indexer.Commit   `json:"commit"`
	Type      string            `json:"string"`
	JoinField map[string]string `json:"join_field"`
}

// Go source is defined to be UTF-8 encoded, so literals here are UTF-8
func TestIndexingTranscodesToUTF8(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildIndex(t)
	defer td()

	require.NoError(t, run("", headSHA))

	for _, tc := range []struct {
		path     string
		expected string
	}{
		{"encoding/iso8859.txt", "狞\n"},                                                         // GB18030
		{"encoding/test.txt", "これはテストです。\nこれもマージして下さい。\n\nAdd excel file.\nDelete excel file."}, // SHIFT_JIS
	} {

		blob, err := c.GetBlob(tc.path)
		require.NoError(t, err)

		blobDoc := &document{}
		require.NoError(t, json.Unmarshal(*blob.Source, &blobDoc))

		require.Equal(t, tc.expected, blobDoc.Blob.Content)
	}
}

func TestIndexingGitlabTest(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildIndex(t)
	defer td()

	require.NoError(t, run("", headSHA))

	// Check the indexing of a commit
	commit, err := c.GetCommit(headSHA)
	require.NoError(t, err)
	require.True(t, commit.Found)
	require.Equal(t, "doc", commit.Type)
	require.Equal(t, projectID+"_"+headSHA, commit.Id)
	require.Equal(t, "project_"+projectID, commit.Routing)

	data := make(map[string]interface{})
	require.NoError(t, json.Unmarshal(*commit.Source, &data))

	commitDoc, ok := data["commit"]
	require.True(t, ok)

	date, err := time.Parse("20060102T150405-0700", "20160927T143746+0000")
	require.NoError(t, err)

	require.Equal(
		t,
		map[string]interface{}{
			"type": "commit",
			"sha":  headSHA,
			"author": map[string]interface{}{
				"email": "job@gitlab.com",
				"name":  "Job van der Voort",
				"time":  date.Local().Format("20060102T150405-0700"),
			},
			"committer": map[string]interface{}{
				"email": "job@gitlab.com",
				"name":  "Job van der Voort",
				"time":  date.Local().Format("20060102T150405-0700"),
			},
			"rid":     projectID,
			"message": "Merge branch 'branch-merged' into 'master'\r\n\r\nadds bar folder and branch-test text file to check Repository merged_to_root_ref method\r\n\r\n\r\n\r\nSee merge request !12",
		},
		commitDoc,
	)

	// Check the indexing of a text blob
	blob, err := c.GetBlob("README.md")
	require.NoError(t, err)
	require.True(t, blob.Found)
	require.Equal(t, "doc", blob.Type)
	require.Equal(t, projectID+"_README.md", blob.Id)
	require.Equal(t, "project_"+projectID, blob.Routing)

	data = make(map[string]interface{})
	require.NoError(t, json.Unmarshal(*blob.Source, &data))

	blobDoc, ok := data["blob"]
	require.True(t, ok)
	require.Equal(
		t,
		map[string]interface{}{
			"type":       "blob",
			"language":   "Markdown",
			"path":       "README.md",
			"file_name":  "README.md",
			"oid":        "faaf198af3a36dbf41961466703cc1d47c61d051",
			"rid":        projectID,
			"commit_sha": headSHA,
			"content":    "testme\n======\n\nSample repo for testing gitlab features\n",
		},
		blobDoc,
	)

	// Check that a binary blob isn't indexed
	_, err = c.GetBlob("Gemfile.zip")
	require.Error(t, err)

	// Test that timezones are preserved
	commit, err = c.GetCommit("498214de67004b1da3d820901307bed2a68a8ef6")
	require.NoError(t, err)

	cDoc := &document{}
	require.NoError(t, json.Unmarshal(*commit.Source, &cDoc))

	date, err = time.Parse("20060102T150405-0700", "20160921T181326+0300")
	require.NoError(t, err)
	expectedDate := date.Local().Format("20060102T150405-0700")

	require.Equal(t, expectedDate, cDoc.Commit.Author.Time)
	require.Equal(t, expectedDate, cDoc.Commit.Committer.Time)
}
