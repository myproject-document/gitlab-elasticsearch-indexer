package git

import (
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/correlation"
)

const (
	testFromCommitSHA = "1234567"
	testToCommitSHA   = "7654321"
)

func startUnixSocketListener() (net.Listener, error) {
	tmpfile, err := os.CreateTemp("", "gitaly.*.socket")
	if err != nil {
		return nil, err
	}
	name := tmpfile.Name()
	os.Remove(name)

	listener, err := net.Listen("unix", name)

	if err != nil {
		return nil, err
	}
	return listener, nil
}

func getConfig(address string) *StorageConfig {
	return &StorageConfig{
		TokenVersion: 2,
		Address:      "unix://" + address,
	}
}

func TestNewGitalyClientWithContextID(t *testing.T) {
	r := require.New(t)

	listener, err := startUnixSocketListener()
	r.NoError(err)
	defer listener.Close()

	testConfig := getConfig(listener.Addr().String())

	client, err := NewGitalyClient(testConfig, testFromCommitSHA, testToCommitSHA, "the-correlation-id", "some-random-id")
	r.NoError(err)
	r.NotNil(client)

	r.Equal("the-correlation-id", correlation.ExtractFromContext(client.ctx))
}
