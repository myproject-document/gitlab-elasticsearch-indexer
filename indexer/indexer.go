package indexer

import (
	"fmt"
	"log"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
)

type Submitter interface {
	Index(id elastic.DocumentRef, thing interface{})
	Remove(id elastic.DocumentRef)

	Flush() error
}

type ProjectID int64

func (p ProjectID) Ref() string {
	return fmt.Sprintf("project_%d", p)
}

type CommitID struct {
	ProjectID
	SHA string
}

func (c *CommitID) Ref() string {
	return fmt.Sprintf("%v_%s", c.ProjectID, c.SHA)
}

func (c *CommitID) RoutingRef() string {
	return c.ProjectID.Ref()
}

type BlobID struct {
	ProjectID
	FilePath string
}

func (b *BlobID) Ref() string {
	return fmt.Sprintf("%v_%s", b.ProjectID, b.FilePath)
}

func (b *BlobID) RoutingRef() string {
	return b.ProjectID.Ref()
}

type Indexer struct {
	ProjectID
	git.Repository
	Submitter
}

func (i *Indexer) submitCommit(c *git.Commit) error {
	commit := BuildCommit(c, i.ProjectID)

	joinData := map[string]string{
		"name":   "commit",
		"parent": i.ProjectID.Ref()}

	i.Submitter.Index(commit.ID, map[string]interface{}{"commit": commit, "type": "commit", "join_field": joinData})
	return nil
}

func (i *Indexer) submitRepoBlob(f *git.File, _, toCommit string) error {
	commitID := CommitID{ i.ProjectID, toCommit }
	blob, err := BuildBlob(f, commitID, "blob")

	if err != nil {
		if isSkipBlobErr(err) {
			return nil
		}

		return fmt.Errorf("Blob %s: %s", f.Path, err)
	}

	joinData := map[string]string{
		"name":   "blob",
		"parent": i.ProjectID.Ref()}

	i.Submitter.Index(blob.ID, map[string]interface{}{"project_id": i.ProjectID, "blob": blob, "type": "blob", "join_field": joinData})
	return nil
}

func (i *Indexer) submitWikiBlob(f *git.File, _, toCommit string) error {
	commitID := CommitID{ i.ProjectID, toCommit }
	wikiBlob, err := BuildBlob(f, commitID, "wiki_blob")
	if err != nil {
		if isSkipBlobErr(err) {
			return nil
		}

		return fmt.Errorf("WikiBlob %s: %s", f.Path, err)
	}

	joinData := map[string]string{
		"name":   "wiki_blob",
		"parent": i.ProjectID.Ref()}

	i.Submitter.Index(wikiBlob.ID, map[string]interface{}{
		"project_id": i.ProjectID,
		"blob": wikiBlob,
		"type": "wiki_blob",
		"join_field": joinData})
	return nil
}

func (i *Indexer) removeBlob(path string) error {
	blobID := BlobID{ i.ProjectID, path }

	i.Submitter.Remove(&blobID)
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
