package testhelpers

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	gitalyClient "gitlab.com/gitlab-org/gitaly/client"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

type gitalyConnectionInfo struct {
	Address string `json:"address"`
	Storage string `json:"storage"`
}

var (
	GitalyConnInfo    *gitalyConnectionInfo
	NamespaceService  pb.NamespaceServiceClient
	RepositoryService pb.RepositoryServiceClient
)

type TestRepository struct {
	Path          string
	RepositoryUrl string
	Namespace     string
}

const (
	ProjectID         = indexer.ProjectID(667)
	ProjectIDString   = "667"
	HeadSHA           = "b83d6e391c22777fca1ed3012fce84f633d7fed0"
	InitialSHA        = "1a0b36b3cdad1d2ee32457c102a8c0b7056fa863"
	TestRepoPath      = "https://gitlab.com/gitlab-org/gitlab-test.git"
	TestRepoNamespace = "test-gitlab-elasticsearch-indexer"
	TestRepo          = "gitlab-test"
)

func init() {
	gci, exists := os.LookupEnv("GITALY_CONNECTION_INFO")
	if exists {
		json.Unmarshal([]byte(gci), &GitalyConnInfo)
	}
}

func EnsureGitalyRepository(t *testing.T) (*pb.Repository, error) {
	return EnsureGitalyRepositoryInNamespace(t, TestRepoNamespace)
}

func EnsureGitalyRepositoryInNamespace(t *testing.T, namespace string) (*pb.Repository, error) {
	conn, err := gitalyClient.Dial(GitalyConnInfo.Address, gitalyClient.DefaultDialOpts)
	if err != nil {
		return nil, fmt.Errorf("did not connect: %s", err)
	}

	NamespaceService = pb.NewNamespaceServiceClient(conn)
	RepositoryService = pb.NewRepositoryServiceClient(conn)

	// Remove the repository if it already exists, for consistency
	rmNsReq := &pb.RemoveNamespaceRequest{
		StorageName: GitalyConnInfo.Storage,
		Name:        namespace,
	}

	_, err = NamespaceService.RemoveNamespace(context.Background(), rmNsReq)
	require.NoError(t, err)

	repository := &pb.Repository{
		StorageName:  GitalyConnInfo.Storage,
		RelativePath: fmt.Sprintf("%s/%s.git", namespace, TestRepo),
	}

	createReq := &pb.CreateRepositoryFromURLRequest{
		Repository: repository,
		Url:        TestRepoPath,
	}

	_, err = RepositoryService.CreateRepositoryFromURL(context.Background(), createReq)
	require.NoError(t, err)

	require.NoError(t, ResetHead(repository, HeadSHA))

	return repository, nil
}

func ResetHead(repository *pb.Repository, toSHA string) error {
	writeHeadReq := &pb.WriteRefRequest{
		Repository: repository,
		Ref:        []byte("refs/heads/master"),
		Revision:   []byte(toSHA),
	}

	_, err := RepositoryService.WriteRef(context.Background(), writeHeadReq)

	return err
}
