package main_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"

	. "gitlab.com/gitlab-org/gitlab-elasticsearch-indexer"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
	H "gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/testhelpers"
)

const (
	BatchSize = 3
)

func TestIndexOperations(t *testing.T) {
	repositories := make([]*pb.Repository, BatchSize)
	operations := make(map[string]*indexer.IndexOperation, BatchSize)

	for i := 0; i < BatchSize; i++ {
		repository, err := H.EnsureGitalyRepositoryInNamespace(t, fmt.Sprintf("ns_%d", i))
		require.NoError(t, err)

		repositories[i] = repository
		operations[repository.RelativePath] = &indexer.IndexOperation{
			ProjectID:      indexer.ProjectID(1 + i),
			RepositoryPath: repository.RelativePath,
			BlobType:       "blob",
			SkipCommits:    false,
		}
	}

	stats := Index(operations)

	require.Equal(t, int64(1), stats.Flushed)
	require.Equal(t, int64(0), stats.Failed)

	for _, operation := range operations {
		require.Equal(t, H.HeadSHA, operation.BaseSHA)
	}

	require.NoError(t, nil)
}
