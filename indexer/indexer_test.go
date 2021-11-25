package indexer_test

import (
	"fmt"
	"io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

const (
	sha            = "9876543210987654321098765432109876543210"
	oid            = "0123456789012345678901234567890123456789"
	parentID       = int64(667)
	parentIDString = "667"
)

func setupEncoder() *indexer.Encoder {
	return indexer.NewEncoder(1024 * 1024)
}

type fakeSubmitter struct {
	flushed int

	indexed      int
	indexedID    []string
	indexedThing []interface{}

	removed   int
	removedID []string
}

type fakeRepository struct {
	commits []*git.Commit

	added    []*git.File
	modified []*git.File
	removed  []*git.File
}

func (f *fakeSubmitter) ParentID() int64 {
	return parentID
}

func (f *fakeSubmitter) Index(id string, thing interface{}) {
	f.indexed++
	f.indexedID = append(f.indexedID, id)
	f.indexedThing = append(f.indexedThing, thing)
}

func (f *fakeSubmitter) Remove(id string) {
	f.removed++
	f.removedID = append(f.removedID, id)
}

func (f *fakeSubmitter) Flush() error {
	f.flushed++
	return nil
}

func (r *fakeRepository) EachFileChange(put git.PutFunc, del git.DelFunc) error {
	for _, file := range r.added {
		if err := put(file, sha, sha); err != nil {
			return err
		}
	}

	for _, file := range r.modified {
		if err := put(file, sha, sha); err != nil {
			return err
		}
	}

	for _, file := range r.removed {
		if err := del(file.Path); err != nil {
			return err
		}
	}

	return nil
}

func (r *fakeRepository) EachCommit(f git.CommitFunc) error {
	for _, commit := range r.commits {
		if err := f(commit); err != nil {
			return err
		}
	}

	return nil
}
func (r *fakeRepository) GetLimitFileSize() int64 {
	return 1024 * 1024
}

func setupIndexer() (*indexer.Indexer, *fakeRepository, *fakeSubmitter) {
	repo := &fakeRepository{}
	submitter := &fakeSubmitter{}

	return indexer.NewIndexer(repo, submitter), repo, submitter
}

func readerFunc(data string, err error) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(data)), err
	}
}

func gitFile(path, content string) *git.File {
	return &git.File{
		Path:         path,
		Blob:         readerFunc(content, nil),
		SkipTooLarge: false,
		Oid:          oid,
	}
}

func gitCommit(message string) *git.Commit {
	return &git.Commit{
		Author: git.Signature{
			Email: "job@gitlab.com",
			Name:  "Job van der Voort",
			When:  time.Date(2016, time.September, 27, 14, 37, 46, 0, time.UTC),
		},
		Committer: git.Signature{
			Email: "nick@gitlab.com",
			Name:  "Nick Thomas",
			When:  time.Date(2017, time.October, 28, 15, 38, 47, 1, time.UTC),
		},
		Message: message,
		Hash:    sha,
	}
}

func validBlob(file *git.File, content, language string) *indexer.Blob {
	return &indexer.Blob{
		Type:      "blob",
		ID:        indexer.GenerateBlobID(parentID, file.Path),
		OID:       oid,
		RepoID:    parentIDString,
		CommitSHA: sha,
		Content:   content,
		Path:      file.Path,
		Filename:  path.Base(file.Path),
		Language:  language,
	}
}

func validCommit(gitCommit *git.Commit) *indexer.Commit {
	return &indexer.Commit{
		Type:      "commit",
		ID:        indexer.GenerateCommitID(parentID, gitCommit.Hash),
		Author:    indexer.BuildPerson(gitCommit.Author, setupEncoder()),
		Committer: indexer.BuildPerson(gitCommit.Committer, setupEncoder()),
		RepoID:    parentIDString,
		Message:   gitCommit.Message,
		SHA:       sha,
	}
}

func index(idx *indexer.Indexer) error {
	if err := idx.IndexBlobs("blob"); err != nil {
		return err
	}

	if err := idx.IndexCommits(); err != nil {
		return err
	}

	if err := idx.Flush(); err != nil {
		return err
	}

	return nil
}

func TestIndex(t *testing.T) {
	idx, repo, submit := setupIndexer()

	gitCommit := gitCommit("Initial commit")
	gitAdded := gitFile("foo/bar", "added file")
	gitModified := gitFile("foo/baz", "modified file")
	gitRemoved := gitFile("foo/qux", "removed file")

	gitTooBig := gitFile("invalid/too-big", "")
	gitTooBig.SkipTooLarge = true

	gitBinary := gitFile("nodisplay/binary.ninja", "foo\x00")

	commit := validCommit(gitCommit)
	added := validBlob(gitAdded, "added file", "Text")

	// If the content is binary, no results (Text) will be returned. This matches the
	// behavior of Linguist.detect: https://github.com/github/linguist/blob/aad49acc0624c70d654a8dce447887dbbc713c7a/lib/linguist.rb#L14-L49
	binary := validBlob(gitBinary, indexer.NoCodeContentMsgHolder, "Text")

	modified := validBlob(gitModified, "modified file", "Text")
	removed := validBlob(gitRemoved, "removed file", "Text")
	tooBig := validBlob(gitTooBig, "", "Text")

	repo.commits = append(repo.commits, gitCommit)
	repo.added = append(repo.added, gitAdded, gitTooBig, gitBinary)
	repo.modified = append(repo.modified, gitModified)
	repo.removed = append(repo.removed, gitRemoved)

	join_data_blob := map[string]string{"name": "blob", "parent": "project_" + parentIDString}
	join_data_commit := map[string]string{"name": "commit", "parent": "project_" + parentIDString}

	err := index(idx)

	require.NoError(t, err)

	require.Equal(t, 5, submit.indexed)
	require.Equal(t, 1, submit.removed)

	require.Equal(t, parentIDString+"_"+added.Path, submit.indexedID[0])
	require.Equal(t, map[string]interface{}{"project_id": parentID, "blob": added, "join_field": join_data_blob, "type": "blob"}, submit.indexedThing[0])

	require.Equal(t, parentIDString+"_"+tooBig.Path, submit.indexedID[1])
	require.Equal(t, map[string]interface{}{"project_id": parentID, "blob": tooBig, "join_field": join_data_blob, "type": "blob"}, submit.indexedThing[1])

	require.Equal(t, parentIDString+"_"+binary.Path, submit.indexedID[2])
	require.Equal(t, map[string]interface{}{"project_id": parentID, "blob": binary, "join_field": join_data_blob, "type": "blob"}, submit.indexedThing[2])

	require.Equal(t, parentIDString+"_"+modified.Path, submit.indexedID[3])
	require.Equal(t, map[string]interface{}{"project_id": parentID, "blob": modified, "join_field": join_data_blob, "type": "blob"}, submit.indexedThing[3])

	require.Equal(t, parentIDString+"_"+commit.SHA, submit.indexedID[4])
	require.Equal(t, map[string]interface{}{"commit": commit, "join_field": join_data_commit, "type": "commit"}, submit.indexedThing[4])

	require.Equal(t, parentIDString+"_"+removed.Path, submit.removedID[0])

	require.Equal(t, submit.flushed, 1)
}

func TestErrorIndexingSkipsRemainder(t *testing.T) {
	idx, repo, submit := setupIndexer()

	gitOKFile := gitFile("ok", "")

	gitBreakingFile := gitFile("broken", "")
	gitBreakingFile.Blob = readerFunc("", fmt.Errorf("Error"))

	repo.added = append(repo.added, gitBreakingFile, gitOKFile)

	err := index(idx)

	require.Error(t, err)
	require.Equal(t, submit.indexed, 0)
	require.Equal(t, submit.removed, 0)
	require.Equal(t, submit.flushed, 0)
}
