package indexer

import (
	"fmt"
	"log"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
)

type Submitter interface {
	ParentID() int64
	ProjectPermissions() *ProjectPermissions

	Index(id string, thing interface{})
	Remove(id string)

	Flush() error
}

type Indexer struct {
	git.Repository
	Submitter
	Encoder *Encoder
}

type ProjectPermissions struct {
	VisibilityLevel       int8
	RepositoryAccessLevel int8
}

func NewIndexer(repository git.Repository, submitter Submitter) *Indexer {
	return &Indexer{
		Repository: repository,
		Submitter:  submitter,
		Encoder:    NewEncoder(repository.GetLimitFileSize()),
	}
}

func (i *Indexer) submitCommit(c *git.Commit) error {
	commit := BuildCommit(c, i.Submitter.ParentID(), i.Encoder)

	joinData := map[string]string{
		"name":   "commit",
		"parent": fmt.Sprintf("project_%v", i.Submitter.ParentID()),
	}

	commitBody := map[string]interface{}{"commit": commit, "type": "commit", "join_field": joinData}

	if permissions := i.Submitter.ProjectPermissions(); permissions != nil {
		commitBody["visibility_level"] = permissions.VisibilityLevel
		commitBody["repository_access_level"] = permissions.RepositoryAccessLevel
	}
	i.Submitter.Index(commit.ID, commitBody)
	return nil
}

func (i *Indexer) submitRepoBlob(f *git.File, _, toCommit string) error {
	blob, err := BuildBlob(f, i.Submitter.ParentID(), toCommit, "blob", i.Encoder)
	if err != nil {
		return fmt.Errorf("Blob %s: %s", f.Path, err)
	}

	joinData := map[string]string{
		"name":   "blob",
		"parent": fmt.Sprintf("project_%v", i.Submitter.ParentID())}

	i.Submitter.Index(blob.ID, map[string]interface{}{"project_id": i.Submitter.ParentID(), "blob": blob, "type": "blob", "join_field": joinData})
	return nil
}

func (i *Indexer) submitWikiBlob(f *git.File, _, toCommit string) error {
	wikiBlob, err := BuildBlob(f, i.Submitter.ParentID(), toCommit, "wiki_blob", i.Encoder)
	if err != nil {
		return fmt.Errorf("WikiBlob %s: %s", f.Path, err)
	}

	joinData := map[string]string{
		"name":   "wiki_blob",
		"parent": fmt.Sprintf("project_%v", i.Submitter.ParentID())}

	i.Submitter.Index(wikiBlob.ID, map[string]interface{}{"project_id": i.Submitter.ParentID(), "blob": wikiBlob, "type": "wiki_blob", "join_field": joinData})
	return nil
}

func (i *Indexer) removeBlob(path string) error {
	blobID := GenerateBlobID(i.Submitter.ParentID(), path)

	i.Submitter.Remove(blobID)
	return nil
}

func (i *Indexer) indexCommits() error {
	return i.Repository.EachCommit(i.submitCommit)
}

func (i *Indexer) indexRepoBlobs() error {
	return i.Repository.EachFileChange(i.submitRepoBlob, i.removeBlob)
}

func (i *Indexer) indexWikiBlobs() error {
	return i.Repository.EachFileChange(i.submitWikiBlob, i.removeBlob)
}

func (i *Indexer) Flush() error {
	return i.Submitter.Flush()
}

func (i *Indexer) IndexBlobs(blobType string) error {
	switch blobType {
	case "blob":
		return i.indexRepoBlobs()
	case "wiki_blob":
		return i.indexWikiBlobs()
	}

	return fmt.Errorf("Unknown blob type: %v", blobType)
}

func (i *Indexer) IndexCommits() error {
	if err := i.indexCommits(); err != nil {
		log.Print("Error while indexing commits: ", err)
		return err
	}

	return nil
}
