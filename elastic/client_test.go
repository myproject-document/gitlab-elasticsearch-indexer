package elastic_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gitlab.com/gitlab-org/es-git-go/elastic"
)

const (
	projectID = "667"
)

func TestAWSConfiguration(t *testing.T) {
	var req *http.Request

	// httptest certificate is unsigned
	transport := http.DefaultTransport
	defer func() { http.DefaultTransport = transport }()
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}

	f := func(w http.ResponseWriter, r *http.Request) {
		req = r

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(f))
	defer srv.Close()

	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["` + srv.URL + `"],
			"aws":true,
			"aws_region": "us-east-1",
			"aws_access_key": "0",
			"aws_secret_access_key": "0"
		}`,
	))
	assert.NoError(t, err)

	client, err := elastic.NewClient(config)
	assert.NoError(t, err)
	defer client.Close()

	if assert.NotNil(t, req) {
		authRE := regexp.MustCompile(`\AAWS4-HMAC-SHA256 Credential=0/\d{8}/us-east-1/es/aws4_request, SignedHeaders=accept;date;host;x-amz-date, Signature=[a-f0-9]{64}\z`)
		assert.Regexp(t, authRE, req.Header.Get("Authorization"))
		assert.NotEqual(t, "", req.Header.Get("X-Amz-Date"))
	}
}

func TestElasticClientIndexAndRetrieval(t *testing.T) {
	config := os.Getenv("ELASTIC_CONNECTION_INFO")
	if config == "" {
		t.Log("ELASTIC_CONNECTION_INFO not set")
		t.SkipNow()
	}

	os.Setenv("RAILS_ENV", fmt.Sprintf("test-elastic-%d", time.Now().Unix()))

	client, err := elastic.FromEnv(projectID)
	assert.NoError(t, err)

	assert.Equal(t, projectID, client.ParentID())

	assert.NoError(t, client.CreateIndex())

	blobDoc := map[string]interface{}{}
	client.Index(projectID+"_foo", blobDoc)

	commitDoc := map[string]interface{}{}
	client.Index(projectID+"_0000", commitDoc)

	assert.NoError(t, client.Flush())

	blob, err := client.GetBlob("foo")
	assert.NoError(t, err)
	assert.Equal(t, true, blob.Found)

	commit, err := client.GetCommit("0000")
	assert.NoError(t, err)
	assert.Equal(t, true, commit.Found)

	client.Remove(projectID + "_foo")
	assert.NoError(t, client.Flush())

	_, err = client.GetBlob("foo")
	assert.Error(t, err)

	assert.NoError(t, client.DeleteIndex())
}
