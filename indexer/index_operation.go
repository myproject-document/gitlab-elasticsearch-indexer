package indexer

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type IndexOperation struct {
	ProjectID      ProjectID
	RepositoryPath string
	BaseSHA        string
	BlobType       string
	SkipCommits    bool
	ErrorCode      int64
}

func (i *IndexOperation) Serialize() string {
	return strings.Join(
		[]string{strconv.FormatInt(int64(i.ProjectID), 10), i.RepositoryPath, i.BaseSHA, strconv.FormatInt(i.ErrorCode, 10)},
		"\t",
	)
}

type IndexOperationReader struct {
	io.Reader
	BlobType    string
	SkipCommits bool
}

func (reader *IndexOperationReader) ReadAllInto(operations map[string]*IndexOperation) (int, error) {
	count := 0
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")

		if l := len(parts); l != 3 {
			return count, fmt.Errorf("Missing arguments, expected 3 got %d", l)
		}

		repositoryPath := parts[1]
		if _, exists := operations[repositoryPath]; exists {
			return count, fmt.Errorf("Repositories can only be specified once, found '%s' multiple times", repositoryPath)
		}

		projectID, err := ParseProjectID(&parts[0])
		if err != nil {
			return count, err
		}

		operations[repositoryPath] = &IndexOperation{
			ProjectID:      projectID,
			RepositoryPath: repositoryPath,
			BaseSHA:        parts[2],
			BlobType:       reader.BlobType,
			SkipCommits:    reader.SkipCommits,
		}
		count++
	}

	return count, nil
}
