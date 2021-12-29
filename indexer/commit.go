package indexer

import (
	"fmt"
	"strconv"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
)

type Commit struct {
	Type      string  `json:"type"`
	ID        string  `json:"-"`
	Author    *Person `json:"author"`
	Committer *Person `json:"committer"`
	RepoID    string  `json:"rid"`
	Message   string  `json:"message"`
	SHA       string  `json:"sha"`
}

func GenerateCommitID(parentID int64, commitSHA string) string {
	return fmt.Sprintf("%v_%s", parentID, commitSHA)
}

func (i *Indexer) BuildCommit(c *git.Commit) *Commit {
	sha := c.Hash

	return &Commit{
		Type:      "commit",
		Author:    BuildPerson(c.Author, i.Encoder),
		Committer: BuildPerson(c.Committer, i.Encoder),
		ID:        GenerateCommitID(i.Submitter.ParentID(), sha),
		RepoID:    strconv.FormatInt(i.Submitter.ParentID(), 10),
		Message:   i.Encoder.tryEncodeString(c.Message),
		SHA:       sha,
	}
}
