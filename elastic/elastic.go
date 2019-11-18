package elastic

import (
	"encoding/json"
	"io"
)

type Config struct {
	IndexName string   `json:"index_name"`
	ProjectID int64    `json:"-"`
	URL       []string `json:"url"`
	AWS       bool     `json:"aws"`
	Region    string   `json:"aws_region"`
	AccessKey string   `json:"aws_access_key"`
	SecretKey string   `json:"aws_secret_access_key"`

	FieldNameTable map[string]map[string]string `json:"transform_tables"`
}

func ReadConfig(r io.Reader) (*Config, error) {
	var out Config

	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return nil, err
	}

	// Fallback if transform_table is not provided
	if out.FieldNameTable == nil {
		out.FieldNameTable = FallbackFieldNameTable()
	}

	return &out, nil
}

func FallbackFieldNameTable() map[string]map[string]string {
	return map[string]map[string]string{
		"blob":   FallbackBlobFieldNameTable(),
		"commit": FallbackCommitFieldNameTable(),
	}
}

func FallbackCommitFieldNameTable() map[string]string {
	return map[string]string{
		"Type":      "type",
		"Author":    "author",
		"Committer": "committer",
		"RepoID":    "rid",
		"Message":   "message",
		"SHA":       "sha",
	}
}

func FallbackBlobFieldNameTable() map[string]string {
	return map[string]string{
		"Type":      "type",
		"OID":       "oid",
		"RepoID":    "rid",
		"CommitSHA": "commit_sha",
		"Content":   "content",
		"Path":      "path",
		"Filename":  "file_name",
		"Language":  "language",
	}
}
