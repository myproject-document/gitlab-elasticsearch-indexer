package indexer

import (
	"strconv"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
)

type Commit struct {
	ID        *CommitID `json:"-"`
	Type      string    `json:"type"`
	Author    *Person   `json:"author"`
	Committer *Person   `json:"committer"`
	RepoID    string    `json:"rid"`
	Message   string    `json:"message"`
	SHA       string    `json:"sha"`
}

func BuildCommit(c *git.Commit, parentID ProjectID) *Commit {
	sha := c.Hash

	return &Commit{
		ID:        &CommitID{parentID, sha},
		Type:      "commit",
		Author:    BuildPerson(c.Author),
		Committer: BuildPerson(c.Committer),
		RepoID:    strconv.FormatInt(int64(parentID), 10),
		Message:   tryEncodeString(c.Message),
		SHA:       sha,
	}
}
