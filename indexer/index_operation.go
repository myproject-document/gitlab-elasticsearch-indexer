package indexer

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)


type IndexOperation struct {
	ProjectID ProjectID
	RepositoryPath string
	BaseSHA string
	BlobType string
	SkipCommits bool
	ErrorCode int64
}

type IndexOperationReader struct {
	io.Reader
	BlobType string
	SkipCommits bool
}

func (i *IndexOperation) Serialize() string {
	return strings.Join(
		[]string{strconv.FormatInt(int64(i.ProjectID), 10), i.RepositoryPath, i.BaseSHA, strconv.FormatInt(i.ErrorCode, 10)},
		"\t",
	)
}

func (reader *IndexOperationReader) ReadOperations() (map[string]*IndexOperation, error) {
	operations := make(map[string]*IndexOperation)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")

		if l := len(parts); l != 3 {
			return nil, fmt.Errorf("Missing arguments, expected 3 got %d", l) 
		}

		repositoryPath := parts[1]
		if _, exists := operations[repositoryPath]; exists {
			return nil, fmt.Errorf("Repositories can only be specified once, found '%s' multiple times", repositoryPath)
		}

		projectID, err := ParseProjectID(&parts[0])
		if err != nil {
			return nil, err
		}

		operation := &IndexOperation{
			ProjectID: projectID,
			RepositoryPath: repositoryPath,
			BaseSHA: parts[2],
			BlobType: reader.BlobType,
			SkipCommits: reader.SkipCommits,
		}

		operations[repositoryPath] = operation
	}

	return operations, nil
}
