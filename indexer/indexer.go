package indexer

import (
	"fmt"
	"log"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
)

type Submitter interface {
	ParentID() string

	Index(id string, thing interface{})
	Remove(id string)

	Flush() error
}

type Indexer struct {
	git.Repository
	Submitter
}

func (i *Indexer) SubmitCommit(c *git.Commit) error {
	commit := BuildCommit(c, i.Submitter.ParentID())

	//	log.Print("Submitting commit: ", commit.Hash)

	i.Submitter.Index(commit.ID, map[string]interface{}{"commit": commit})
	return nil
}

func (i *Indexer) SubmitBlob(f *git.File, _, toCommit string) error {
	blob, err := BuildBlob(f, i.Submitter.ParentID(), toCommit)
	if err != nil {
		if isSkipBlobErr(err) {
			return nil
		}

		return fmt.Errorf("Blob %s: %s", f.Path, err)
	}

	//	log.Print("Submitting blob: ", f.Path)

	i.Submitter.Index(blob.ID, map[string]interface{}{"blob": blob})
	return nil
}

func (i *Indexer) RemoveBlob(file *git.File, _, _ string) error {
	blobID := GenerateBlobID(i.Submitter.ParentID(), file.Path)

	//	log.Print("Removing blob: ", file.Path)

	i.Submitter.Remove(blobID)
	return nil
}

func (i *Indexer) IndexCommits() error {
	return i.Repository.EachCommit(i.SubmitCommit)
}

func (i *Indexer) IndexBlobs() error {
	return i.Repository.EachFileChange(i.SubmitBlob, i.SubmitBlob, i.RemoveBlob)
}

func (i *Indexer) Index() error {
	if err := i.IndexBlobs(); err != nil {
		log.Print("Error while indexing blobs: ", err)
		return err
	}

	if err := i.IndexCommits(); err != nil {
		log.Print("Error while indexing commits: ", err)
		return err
	}

	if err := i.Submitter.Flush(); err != nil {
		log.Print("Error while flushing requests: ", err)
	}

	return nil
}
