package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitalyClient "gitlab.com/gitlab-org/gitaly/v14/client"
	pb "gitlab.com/gitlab-org/gitaly/v14/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

var (
	binary         = flag.String("binary", "./bin/gitlab-elasticsearch-indexer", "Path to `gitlab-elasticsearch-indexer` binary for integration tests")
	gitalyConnInfo *gitalyConnectionInfo
)

const (
	projectID             = 667
	projectIDString       = "667"
	headSHA               = "b83d6e391c22777fca1ed3012fce84f633d7fed0"
	testRepo              = "test-gitlab-elasticsearch-indexer/gitlab-test.git"
	testRepoPath          = "https://gitlab.com/gitlab-org/gitlab-test.git"
	testRepoNamespace     = "test-gitlab-elasticsearch-indexer"
	visibilityLevel       = int8(10)
	repositoryAccessLevel = int8(20)
)

type gitalyConnectionInfo struct {
	Address string `json:"address"`
	Storage string `json:"storage"`
}

func validProjectPermissions() *indexer.ProjectPermissions {
	return &indexer.ProjectPermissions{
		VisibilityLevel:       visibilityLevel,
		RepositoryAccessLevel: repositoryAccessLevel,
	}
}

func init() {
	gci, exists := os.LookupEnv("GITALY_CONNECTION_INFO")
	if exists {
		err := json.Unmarshal([]byte(gci), &gitalyConnInfo)
		if err != nil {
			panic(err)
		}
	}
}

func TestIndexingRenamesFiles(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildWorkingIndex(t)

	defer td()

	// The commit before files/js/commit.js.coffee is renamed
	err, _, _ := run("", "281d3a76f31c812dbf48abce82ccf6860adedd81")
	require.NoError(t, err)
	_, err = c.GetBlob("files/js/commit.js.coffee")
	require.NoError(t, err)

	// Now we expect it to have been renamed
	err, _, _ = run("281d3a76f31c812dbf48abce82ccf6860adedd81", "c347ca2e140aa667b968e51ed0ffe055501fe4f4")
	require.NoError(t, err)
	_, err = c.GetBlob("files/js/commit.js.coffee")
	require.Error(t, err)
	_, err = c.GetBlob("files/js/commit.coffee")
	require.NoError(t, err)
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

func buildWorkingIndex(t *testing.T) (*elastic.Client, func()) {
	return buildIndex(t, true)
}

func buildBrokenIndex(t *testing.T) (*elastic.Client, func()) {
	return buildIndex(t, false)
}

func buildIndex(t *testing.T, working bool) (*elastic.Client, func()) {
	setElasticsearchConnectionInfo(t)

	config, err := elastic.ConfigFromEnv()
	require.NoError(t, err)

	config.Permissions = validProjectPermissions()
	config.ProjectID = projectID

	client, err := elastic.NewClient(config, "the-correlation-id")
	require.NoError(t, err)

	if working {
		require.NoError(t, client.CreateWorkingIndex())
	} else {
		require.NoError(t, client.CreateBrokenIndex())
	}

	return client, func() {
		err := client.DeleteIndex()
		if err != nil {
			require.NoError(t, err)
		}
	}
}

// Substitude index_name with a dynamically generated one
func setElasticsearchConnectionInfo(t *testing.T) {
	config, err := elastic.ReadConfig(strings.NewReader(os.Getenv("ELASTIC_CONNECTION_INFO")))
	require.NoError(t, err)

	config.IndexName = fmt.Sprintf("%s-%d", config.IndexName, time.Now().Unix())
	out, err := json.Marshal(config)
	require.NoError(t, err)

	os.Setenv("ELASTIC_CONNECTION_INFO", string(out))
}

func run(from, to string, args ...string) (error, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	arguments := append(args, projectIDString, testRepo)
	cmd := exec.Command(*binary, arguments...)
	cmd.Env = os.Environ()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// GitLab always sets FROM_SHA
	if from == "" {
		from = "0000000000000000000000000000000000000000"
	}

	cmd.Env = append(cmd.Env, "FROM_SHA="+from)

	if to != "" {
		cmd.Env = append(cmd.Env, "TO_SHA="+to)
	}

	err := cmd.Run()

	return err, stdout.String(), stderr.String()
}

func TestIndexingRemovesFiles(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildWorkingIndex(t)

	defer td()

	// The commit before files/empty is removed - so it should be indexed
	err, _, _ := run("", "19e2e9b4ef76b422ce1154af39a91323ccc57434")
	require.NoError(t, err)
	_, err = c.GetBlob("files/empty")
	require.NoError(t, err)

	// Now we expect it to have been removed
	err, _, _ = run("19e2e9b4ef76b422ce1154af39a91323ccc57434", "08f22f255f082689c0d7d39d19205085311542bc")
	require.NoError(t, err)
	_, err = c.GetBlob("files/empty")
	require.Error(t, err)
}

func TestIndexingTimeout(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	_, td := buildWorkingIndex(t)

	defer td()

	err, stdout, _ := run("", "e2c7507b72f55cc272bbd5fde5bfa46eb4aeeebf", "--timeout=0s")

	require.Error(t, err)
	require.Regexp(t, `The process has timed out after 0s`, stdout)

	err, stdout, _ = run("", "e2c7507b72f55cc272bbd5fde5bfa46eb4aeeebf", "--timeout=100")

	require.Error(t, err)
	require.Regexp(t, `time: missing unit in duration`, stdout)
}

func TestIndexingFilesWithLongPath(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildWorkingIndex(t)

	defer td()

	// commit where a filename with a very long path is introduced
	err, _, _ := run("", "e2c7507b72f55cc272bbd5fde5bfa46eb4aeeebf")
	require.NoError(t, err)

	// hashed filename from the commit above
	_, err = c.GetBlob("e600da5384d3a616836c47fb8df1a84ce7752bd3")
	require.NoError(t, err)
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
	c, td := buildWorkingIndex(t)
	defer td()

	err, _, _ := run("", headSHA)
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		path     string
		expected string
	}{
		{"GB18030", "encoding/iso8859.txt", "狞\n"},
		{"SHIFT_JIS", "encoding/test.txt", "これはテストです。\nこれもマージして下さい。\n\nAdd excel file.\nDelete excel file."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := c.GetBlob(tc.path)
			require.NoError(t, err)

			blobDoc := &document{}
			require.NoError(t, json.Unmarshal(*blob.Source, &blobDoc))

			require.Equal(t, tc.expected, blobDoc.Blob.Content)
		})
	}
}

func TestElasticClientIndexMismatch(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	_, td := buildBrokenIndex(t)
	defer td()

	err, _, stderr := run("", headSHA)

	require.Error(t, err)
	require.Regexp(t, `bulk request \d: failed to insert \d/\d documents`, stderr)
}

func TestIndexingGitlabTest(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildWorkingIndex(t)
	defer td()

	err, _, _ := run("", headSHA)
	require.NoError(t, err)

	// Check the indexing of a commit
	commit, err := c.GetCommit(headSHA)
	require.NoError(t, err)
	require.True(t, commit.Found)
	require.Equal(t, "_doc", commit.Type)
	require.Equal(t, projectIDString+"_"+headSHA, commit.Id)
	require.Equal(t, "project_"+projectIDString, commit.Routing)

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
			"rid":     projectIDString,
			"message": "Merge branch 'branch-merged' into 'master'\r\n\r\nadds bar folder and branch-test text file to check Repository merged_to_root_ref method\r\n\r\n\r\n\r\nSee merge request !12",
		},
		commitDoc,
	)

	// Check the indexing of a text blob
	blob, err := c.GetBlob("README.md")
	require.NoError(t, err)
	require.True(t, blob.Found)
	require.Equal(t, "_doc", blob.Type)
	require.Equal(t, projectIDString+"_README.md", blob.Id)
	require.Equal(t, "project_"+projectIDString, blob.Routing)

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
			"rid":        projectIDString,
			"commit_sha": headSHA,
			"content":    "testme\n======\n\nSample repo for testing gitlab features\n",
		},
		blobDoc,
	)

	// Check that a binary blob is indexed
	blob, err = c.GetBlob("Gemfile.zip")
	require.NoError(t, err)
	require.True(t, blob.Found)
	require.Equal(t, projectIDString+"_Gemfile.zip", blob.Id)
	require.Equal(t, "project_"+projectIDString, blob.Routing)

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

func TestIndexingWikiBlobs(t *testing.T) {
	checkDeps(t)
	ensureGitalyRepository(t)
	c, td := buildWorkingIndex(t)
	defer td()

	err, _, _ := run("", headSHA, "--blob-type=wiki_blob", "--skip-commits")
	require.NoError(t, err)

	// Check that commits were not indexed
	commit, err := c.GetCommit(headSHA)
	require.Error(t, err)
	require.Empty(t, commit)

	// Check that blobs are indexed
	blob, err := c.GetBlob("README.md")
	require.NoError(t, err)
	require.True(t, blob.Found)
	require.Equal(t, "_doc", blob.Type)
	require.Equal(t, projectIDString+"_README.md", blob.Id)
	require.Equal(t, "project_"+projectIDString, blob.Routing)

	data := make(map[string]interface{})
	require.NoError(t, json.Unmarshal(*blob.Source, &data))

	blobDoc, ok := data["blob"]
	require.True(t, ok)
	require.Equal(
		t,
		map[string]interface{}{
			"type":       "wiki_blob",
			"language":   "Markdown",
			"path":       "README.md",
			"file_name":  "README.md",
			"oid":        "faaf198af3a36dbf41961466703cc1d47c61d051",
			"rid":        fmt.Sprintf("wiki_%s", projectIDString),
			"commit_sha": headSHA,
			"content":    "testme\n======\n\nSample repo for testing gitlab features\n",
		},
		blobDoc,
	)

}
