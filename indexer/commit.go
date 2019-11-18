package indexer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
)

type Commit struct {
	Type      string
	ID        string
	Author    *Person
	Committer *Person
	RepoID    string
	Message   string
	SHA       string

	FieldNameTable map[string]string
}

func GenerateCommitID(parentID int64, commitSHA string) string {
	return fmt.Sprintf("%v_%s", parentID, commitSHA)
}

func BuildCommit(c *git.Commit, parentID int64, mapping map[string]string) *Commit {
	sha := c.Hash

	return &Commit{
		Type:      "commit",
		Author:    BuildPerson(c.Author),
		Committer: BuildPerson(c.Committer),
		ID:        GenerateCommitID(parentID, sha),
		RepoID:    strconv.FormatInt(parentID, 10),
		Message:   tryEncodeString(c.Message),
		SHA:       sha,

		FieldNameTable: mapping,
	}
}

func (c *Commit) MarshalJSON() ([]byte, error) {
	out := map[string]interface{}{}

	s := reflect.ValueOf(c).Elem()
	typeOfT := s.Type()
	num := s.NumField()

	for i := 0; i < num; i++ {
		f := s.Field(i)

		key := typeOfT.Field(i).Name
		if newKey, ok := c.FieldNameTable[key]; ok {
			out[newKey] = f.Interface()
		}
	}

	return json.Marshal(out)
}
