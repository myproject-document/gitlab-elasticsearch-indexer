package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	logkit "gitlab.com/gitlab-org/labkit/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	gitalyauth "gitlab.com/gitlab-org/gitaly/v14/auth"
	gitalyclient "gitlab.com/gitlab-org/gitaly/v14/client"
	pb "gitlab.com/gitlab-org/gitaly/v14/proto/go/gitalypb"
	"gitlab.com/gitlab-org/labkit/correlation"
	grpccorrelation "gitlab.com/gitlab-org/labkit/correlation/grpc"
)

const (
	SubmoduleFileMode = 0160000
	// See https://stackoverflow.com/questions/9765453/is-gits-semi-secret-empty-tree-object-reliable-and-why-is-there-not-a-symbolic
	NullTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	ZeroSHA     = "0000000000000000000000000000000000000000"

	clientName                 = "gitlab-elasticsearch-indexer"
	defaultLimitFileSize int64 = 1024 * 1024
)

type StorageConfig struct {
	Address       string `json:"address"`
	Token         string `json:"token"`
	StorageName   string `json:"storage"`
	RelativePath  string `json:"relative_path"`
	ProjectPath   string `json:"project_path"`
	LimitFileSize int64  `json:"limit_file_size"`
	TokenVersion  int    `json:"token_version"`
}

type gitalyClient struct {
	conn                    *grpc.ClientConn
	repository              *pb.Repository
	blobServiceClient       pb.BlobServiceClient
	repositoryServiceClient pb.RepositoryServiceClient
	refServiceClient        pb.RefServiceClient
	commitServiceClient     pb.CommitServiceClient
	ctx                     context.Context
	FromHash                string
	ToHash                  string
	limitFileSize           int64
}

func NewGitalyClient(config *StorageConfig, fromSHA, toSHA, correlationID, projectID string) (*gitalyClient, error) {
	var RPCCred credentials.PerRPCCredentials
	if config.TokenVersion == 0 || config.TokenVersion == 2 {
		RPCCred = gitalyauth.RPCCredentialsV2(config.Token)
	} else {
		return nil, errors.New("Unknown token version")
	}

	connOpts := append(
		gitalyclient.DefaultDialOpts,
		grpc.WithPerRPCCredentials(RPCCred),
		grpc.WithStreamInterceptor(
			grpccorrelation.StreamClientCorrelationInterceptor(
				grpccorrelation.WithClientName(clientName),
			),
		),
		grpc.WithUnaryInterceptor(
			grpccorrelation.UnaryClientCorrelationInterceptor(
				grpccorrelation.WithClientName(clientName),
			),
		),
	)

	ctx := newContext(correlationID)

	conn, err := gitalyclient.Dial(config.Address, connOpts)
	if err != nil {
		return nil, fmt.Errorf("did not connect: %s", err)
	}

	repository := &pb.Repository{
		StorageName:   config.StorageName,
		RelativePath:  config.RelativePath,
		GlProjectPath: config.ProjectPath,
		GlRepository:  projectID,
	}

	client := &gitalyClient{
		conn:                    conn,
		repository:              repository,
		blobServiceClient:       pb.NewBlobServiceClient(conn),
		repositoryServiceClient: pb.NewRepositoryServiceClient(conn),
		refServiceClient:        pb.NewRefServiceClient(conn),
		commitServiceClient:     pb.NewCommitServiceClient(conn),
		ctx:                     ctx,
		limitFileSize:           config.LimitFileSize,
	}

	if fromSHA == "" || fromSHA == ZeroSHA {
		client.FromHash = NullTreeSHA
	} else {
		client.FromHash = fromSHA
	}

	if toSHA == "" {
		head, err := client.lookUpHEAD()
		if err != nil {
			return nil, fmt.Errorf("lookUpHEAD: %v", err)
		}
		client.ToHash = head
	} else {
		client.ToHash = toSHA
	}

	return client, nil
}

func ReadConfig(repoPath, projectPath string) (*StorageConfig, error) {
	data := strings.NewReader(os.Getenv("GITALY_CONNECTION_INFO"))

	config := StorageConfig{
		RelativePath:  repoPath,
		ProjectPath:   projectPath,
		LimitFileSize: defaultLimitFileSize,
	}

	err := json.NewDecoder(data).Decode(&config)

	return &config, err
}

func NewGitalyClientFromEnv(repoPath, fromSHA, toSHA, correlationID, projectID, projectPath string) (*gitalyClient, error) {
	config, err := ReadConfig(repoPath, projectPath)

	if err != nil {
		return nil, err
	}

	client, err := NewGitalyClient(config, fromSHA, toSHA, correlationID, projectID)
	if err != nil {
		return nil, fmt.Errorf("Failed to open %s: %s", config.RelativePath, err)
	}

	return client, nil
}

func (gc *gitalyClient) Close() {
	gc.conn.Close()
}

func (gc *gitalyClient) EachFileChange(put PutFunc, del DelFunc) error {
	request := &pb.GetRawChangesRequest{
		Repository:   gc.repository,
		FromRevision: gc.FromHash,
		ToRevision:   gc.ToHash,
	}

	stream, err := gc.repositoryServiceClient.GetRawChanges(gc.ctx, request)
	if err != nil {
		return fmt.Errorf("could not call rpc.GetRawChanges: %v", err)
	}

	for {
		c, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%v.GetRawChanges, %v", c, err)
		}
		for _, change := range c.RawChanges {
			// TODO: We just skip submodules from indexing now just to mirror the go-git
			// implementation but it can be not that expensive to implement with gitaly actually so some
			// investigation is required here
			if change.OldMode == SubmoduleFileMode || change.NewMode == SubmoduleFileMode {
				continue
			}

			switch change.Operation.String() {
			case "DELETED", "RENAMED":
				path := string(change.OldPathBytes)
				logkit.WithFields(
					logkit.Fields{
						"operation": "DELETE",
						"path":      path,
					},
				).Debug("Indexing blob change")
				if err = del(path); err != nil {
					return err
				}
			}

			switch change.Operation.String() {
			case "ADDED", "RENAMED", "MODIFIED", "COPIED":
				file, err := gc.gitalyBuildFile(change, string(change.NewPathBytes))
				if err != nil {
					return err
				}
				logkit.WithFields(
					logkit.Fields{
						"operation": "PUT",
						"path":      file.Path,
					},
				).Debug("Indexing blob change")
				if err = put(file, gc.FromHash, gc.ToHash); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// HEAD is not always set in some cases, so we find the last commit in
// a default branch instead
func (gc *gitalyClient) lookUpHEAD() (string, error) {
	defaultBranchName, err := gc.findDefaultBranchName()
	if err != nil {
		return "", err
	}

	request := &pb.FindCommitRequest{
		Repository: gc.repository,
		Revision:   defaultBranchName,
	}

	response, err := gc.commitServiceClient.FindCommit(gc.ctx, request)
	if err != nil {
		return "", fmt.Errorf("Cannot look up HEAD: %v", err)
	}
	return response.Commit.Id, nil
}

func (gc *gitalyClient) findDefaultBranchName() ([]byte, error) {
	request := &pb.FindDefaultBranchNameRequest{
		Repository: gc.repository,
	}

	response, err := gc.refServiceClient.FindDefaultBranchName(gc.ctx, request)
	if err != nil {
		return nil, fmt.Errorf("Cannot find a default branch: %v", err)
	}
	return response.Name, nil
}

func (gc *gitalyClient) getBlob(oid string) (io.ReadCloser, error) {
	data := new(bytes.Buffer)

	request := &pb.GetBlobRequest{
		Repository: gc.repository,
		Oid:        oid,
		Limit:      gc.limitFileSize,
	}

	stream, err := gc.blobServiceClient.GetBlob(gc.ctx, request)
	if err != nil {
		return nil, fmt.Errorf("Cannot get blob: %s", oid)
	}

	for {
		c, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%v.GetBlob: %v", c, err)
		}
		if c.Data != nil {
			data.Write(c.Data)
		}
	}

	return io.NopCloser(data), nil
}

func (gc *gitalyClient) gitalyBuildFile(change *pb.GetRawChangesResponse_RawChange, path string) (*File, error) {
	var data io.ReadCloser
	var skipTooLarge bool
	// We limit the size to avoid loading too big blobs into memory
	// as they will be rejected on the indexer side anyway
	// Ideally, we need to create a lazy blob reader here.
	if change.Size > gc.limitFileSize {
		data = io.NopCloser(new(bytes.Buffer))
		skipTooLarge = true
	} else {
		var err error
		data, err = gc.getBlob(change.BlobId)
		if err != nil {
			return nil, fmt.Errorf("getBlob returns error: %v", err)
		}
	}

	return &File{
		Path:         path,
		Oid:          change.BlobId,
		Blob:         getBlobReader(data),
		SkipTooLarge: skipTooLarge,
	}, nil
}

func getBlobReader(data io.ReadCloser) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return data, nil }
}

func (gc *gitalyClient) EachCommit(f CommitFunc) error {
	request := &pb.ListCommitsRequest{
		Repository: gc.repository,
		Revisions: []string{
			"^" + gc.FromHash,
			gc.ToHash,
		},
		Reverse: true,
	}

	stream, err := gc.commitServiceClient.ListCommits(gc.ctx, request)
	if err != nil {
		return fmt.Errorf("could not call rpc.ListCommits: %v", err)
	}

	for {
		c, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error calling rpc.ListCommits: %v", err)
		}
		for _, cmt := range c.Commits {
			commit := &Commit{
				Message:   string(cmt.Body),
				Hash:      string(cmt.Id),
				Author:    gitalyBuildSignature(cmt.Author),
				Committer: gitalyBuildSignature(cmt.Committer),
			}

			logkit.WithField("commitID", cmt.Id).Debug("Indexing commit")

			if err := f(commit); err != nil {
				return err
			}
		}
	}
	return nil
}

func (gc *gitalyClient) GetLimitFileSize() int64 {
	return gc.limitFileSize
}

func gitalyBuildSignature(ca *pb.CommitAuthor) Signature {
	return Signature{
		Name:  string(ca.Name),
		Email: string(ca.Email),
		When:  time.Unix(ca.Date.GetSeconds(), 0), // another option is ptypes.Timestamp(ca.Date)
	}
}

func newContext(correlationID string) context.Context {
	return correlation.ContextWithCorrelation(context.Background(), correlationID)
}
