package git

import (
	"io/ioutil"
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
	tmpfile, err := ioutil.TempFile("", "gitaly.*.socket")
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

	os.Setenv(envCorrelationIDKey, "random-cid")

	client, err := NewGitalyClient(testConfig, testFromCommitSHA, testToCommitSHA)
	r.NoError(err)
	r.NotNil(client)

	r.Equal("random-cid", correlation.ExtractFromContext(client.ctx))
}

func TestNewGitalyClientNoContextID(t *testing.T) {
	r := require.New(t)

	listener, err := startUnixSocketListener()
	r.NoError(err)
	defer listener.Close()

	testConfig := getConfig(listener.Addr().String())

	client, err := NewGitalyClient(testConfig, testFromCommitSHA, testToCommitSHA)
	r.NoError(err)
	r.NotNil(client)

	//Random Correlation ID will be generaated when it's not in the client context
	r.NotEmpty(correlation.ExtractFromContext(client.ctx))
}
