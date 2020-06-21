package main_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
	H "gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/testhelpers"
)

var (
	binary = flag.String("binary", "./bin/gitlab-elasticsearch-indexer", "Path to `gitlab-elasticsearch-indexer` binary for integration tests")
)

func TestIndexingRenamesFiles(t *testing.T) {
	checkDeps(t)
	repository, err := H.EnsureGitalyRepository(t)
	client, cleanup := buildWorkingIndex(t)

	defer cleanup()

	// The commit before files/js/commit.js.coffee is renamed
	H.ResetHead(repository, "281d3a76f31c812dbf48abce82ccf6860adedd81")
	err, _, _ = run(H.InitialSHA)
	require.NoError(t, err)

	_, err = fetchBlob(client, "files/js/commit.js.coffee")
	require.NoError(t, err)

	// The commit that renames files/js/commit.js.coffee → files/js/commit.coffee
	H.ResetHead(repository, "6907208d755b60ebeacb2e9dfea74c92c3449a1f")
	err, _, _ = run("281d3a76f31c812dbf48abce82ccf6860adedd81")
	require.NoError(t, err)

	// Now we expect it to have been renamed
	_, err = fetchBlob(client, "files/js/commit.js.coffee")
	require.Error(t, err)
	_, err = fetchBlob(client, "files/js/commit.coffee")
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

func buildIndex(t *testing.T, working bool) (client *elastic.Client, cleanup func()) {
	setElasticsearchConnectionInfo(t)

	client, err := elastic.FromEnv()
	require.NoError(t, err)
	
	cleanup = func() { require.NoError(t, client.DeleteIndex()) }

	if working {
		require.NoError(t, client.CreateWorkingIndex())
	} else {
		require.NoError(t, client.CreateBrokenIndex())
	}

	return
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

func fetchBlob(c *elastic.Client, path string) (*elastic.Result, error) {
	blobID := indexer.BlobID{
		ProjectID: H.ProjectID,
		FilePath:  path,
	}

	return c.Get(&blobID)
}

func fetchCommit(c *elastic.Client, sha string) (*elastic.Result, error) {
	commitID := indexer.CommitID{
		ProjectID: H.ProjectID,
		SHA:       sha,
	}

	return c.Get(&commitID)
}

func run(from string, args ...string) (error, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	arguments := append(args, H.ProjectIDString, fmt.Sprintf("%s/%s.git", H.TestRepoNamespace, H.TestRepo))
	cmd := exec.Command(*binary, arguments...)
	cmd.Env = os.Environ()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// GitLab always sets FROM_SHA
	if from == "" {
		from = "0000000000000000000000000000000000000000"
	}

	cmd.Env = append(cmd.Env, "FROM_SHA="+from)

	err := cmd.Run()

	if os.Getenv("DEBUG") != "" {
		fmt.Println("=== STDOUT ===")
		fmt.Printf(stdout.String())

		fmt.Println("=== STDERR ===")
		fmt.Printf(stderr.String())
	}

	return err, stdout.String(), stderr.String()
}

func TestIndexingRemovesFiles(t *testing.T) {
	checkDeps(t)
	repository, err := H.EnsureGitalyRepository(t)
	client, cleanup := buildWorkingIndex(t)

	defer cleanup()

	// The commit before files/empty is removed - so it should be indexed
	H.ResetHead(repository, "9a944d90955aaf45f6d0c88f30e27f8d2c41cec0")

	err, _, _ = run(H.InitialSHA)
	require.NoError(t, err)
	_, err = fetchBlob(client, "files/empty")
	require.NoError(t, err)

	// Reset HEAD back to a commit that removes files
	H.ResetHead(repository, "08f22f255f082689c0d7d39d19205085311542bc")

	// Now we expect it to have been removed
	err, _, _ = run("19e2e9b4ef76b422ce1154af39a91323ccc57434")
	require.NoError(t, err)
	_, err = fetchBlob(client, "files/empty")
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
	H.EnsureGitalyRepository(t)
	client, cleanup := buildWorkingIndex(t)

	defer cleanup()

	err, _, _ := run("")
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
			blob, err := fetchBlob(client, tc.path)
			require.NoError(t, err)

			blobDoc := &document{}
			require.NoError(t, json.Unmarshal(*blob.Source, &blobDoc))

			require.Equal(t, tc.expected, blobDoc.Blob.Content)
		})
	}
}

func TestElasticClientIndexMismatch(t *testing.T) {
	checkDeps(t)
	H.EnsureGitalyRepository(t)
	_, cleanup := buildBrokenIndex(t)

	defer cleanup()

	err, _, stderr := run("")

	require.Error(t, err)
	require.Regexp(t, `bulk request \d: failed to insert \d/\d documents`, stderr)
}

func TestIndexingGitlabTest(t *testing.T) {
	checkDeps(t)
	H.EnsureGitalyRepository(t)
	client, cleanup := buildWorkingIndex(t)

	defer cleanup()

	err, _, _ := run("")
	require.NoError(t, err)

	// Check the indexing of a commit
	commit, err := fetchCommit(client, H.HeadSHA)
	require.NoError(t, err)
	require.True(t, commit.Found)
	require.Equal(t, "doc", commit.Type)
	require.Equal(t, H.ProjectIDString+"_"+H.HeadSHA, commit.Id)
	require.Equal(t, "project_"+H.ProjectIDString, commit.Routing)

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
			"sha":  H.HeadSHA,
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
			"rid":     H.ProjectIDString,
			"message": "Merge branch 'branch-merged' into 'master'\r\n\r\nadds bar folder and branch-test text file to check Repository merged_to_root_ref method\r\n\r\n\r\n\r\nSee merge request !12",
		},
		commitDoc,
	)

	// Check the indexing of a text blob
	blob, err := fetchBlob(client, "README.md")
	require.NoError(t, err)
	require.True(t, blob.Found)
	require.Equal(t, "doc", blob.Type)
	require.Equal(t, H.ProjectIDString+"_README.md", blob.Id)
	require.Equal(t, "project_"+H.ProjectIDString, blob.Routing)

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
			"rid":        H.ProjectIDString,
			"commit_sha": H.HeadSHA,
			"content":    "testme\n======\n\nSample repo for testing gitlab features\n",
		},
		blobDoc,
	)

	// Check that a binary blob isn't indexed
	_, err = fetchBlob(client, "Gemfile.zip")
	require.Error(t, err)

	// Test that timezones are preserved
	commit, err = fetchCommit(client, "498214de67004b1da3d820901307bed2a68a8ef6")
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
	H.EnsureGitalyRepository(t)
	client, cleanup := buildWorkingIndex(t)

	defer cleanup()

	err, _, _ := run("", "--blob-type=wiki_blob", "--skip-commits")
	require.NoError(t, err)

	// Check that commits were not indexed
	commit, err := fetchCommit(client, H.HeadSHA)
	require.Error(t, err)
	require.Empty(t, commit)

	// Check that blobs are indexed
	blob, err := fetchBlob(client, "README.md")
	require.NoError(t, err)
	require.True(t, blob.Found)
	require.Equal(t, "doc", blob.Type)
	require.Equal(t, H.ProjectIDString+"_README.md", blob.Id)
	require.Equal(t, "project_"+H.ProjectIDString, blob.Routing)

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
			"rid":        fmt.Sprintf("wiki_%s", H.ProjectIDString),
			"commit_sha": H.HeadSHA,
			"content":    "testme\n======\n\nSample repo for testing gitlab features\n",
		},
		blobDoc,
	)
}

func TestInputFile(t *testing.T) {
	const BatchSize = 3
	const EmptySHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904" // git's empty tree id
	const OperationsFilePath = "/tmp/gitlab-elasticsearch-indexer-input"

	repositories := make([]*pb.Repository, BatchSize)

	for i := 0; i < BatchSize; i++ {
		repository, err := H.EnsureGitalyRepositoryInNamespace(t, fmt.Sprintf("ns_%d", i))
		require.NoError(t, err)

		repositories[i] = repository
	}

	{
		operationsFile, err := os.Create(OperationsFilePath)
		require.NoError(t, err)

		defer operationsFile.Close()
		defer operationsFile.Sync()

		for i, repository := range repositories {
			operationSpec := fmt.Sprintf("%d\t%s\t%s\n", 1+i, repository.RelativePath, EmptySHA)
			_, err := operationsFile.WriteString(operationSpec)
			require.NoError(t, err)
		}

		if os.Getenv("DEBUG") != "" {
			fmt.Println("=== INPUT-FILE ===")
			operationsFile.Seek(0, 0)
			io.Copy(os.Stdout, operationsFile)
		}
	}

	defer os.Remove(OperationsFilePath)

	// let's use this file as `--input-file'
	err, _, _ := run("", "--blob-type=blob", "--skip-commits", fmt.Sprintf("--input-file=%s", OperationsFilePath))
	require.NoError(t, err)
}

func TestStdoutOperationResult(t *testing.T) {
	checkDeps(t)
	H.EnsureGitalyRepository(t)
	_, cleanup := buildWorkingIndex(t)

	defer cleanup()

	err, stdout, _ := run("", "--blob-type=blob", "--skip-commits")
	require.NoError(t, err)

	testRepoPath := fmt.Sprintf("%s/%s.git", H.TestRepoNamespace, H.TestRepo)
	resultSpec := fmt.Sprintf("%d\t%s\t%s\t%d\n", H.ProjectID, testRepoPath, H.HeadSHA, 0)

	require.Equal(t, resultSpec, stdout)
}
