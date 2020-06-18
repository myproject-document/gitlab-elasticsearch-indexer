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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
)

const (
	projectID       = int64(667)
	projectIDString = "667"
)

const credsRespTmpl = `{
  "Code": "Success",
  "Type": "AWS-HMAC",
  "AccessKeyId" : "accessKey",
  "SecretAccessKey" : "secret",
  "Token" : "token",
  "Expiration" : "%s",
  "LastUpdated" : "2009-11-23T0:00:00Z"
}`

const credsFailRespTmpl = `{
  "Code": "ErrorCode",
  "Message": "ErrorMsg",
  "LastUpdated": "2009-11-23T0:00:00Z"
}`

type DocumentID struct {
	RawRef        string
	RawRoutingRef string
}

func (d *DocumentID) Ref() string {
	return d.RawRef
}

func (d *DocumentID) RoutingRef() string {
	return d.RawRoutingRef
}

func initTestServer(expireOn string, failAssume bool) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/meta-data/iam/security-credentials/":
			fmt.Fprintln(w, "RoleName")
		case "/latest/meta-data/iam/security-credentials/RoleName":
			if failAssume {
				fmt.Fprintf(w, credsFailRespTmpl)
			} else {
				fmt.Fprintf(w, credsRespTmpl, expireOn)
			}
		default:
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))

	return server
}

func TestResolveAWSCredentialsStatic(t *testing.T) {
	aws_config := &aws.Config{}
	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["http://localhost:9200"],
			"aws":true,
			"aws_access_key": "static_access_key",
			"aws_secret_access_key": "static_secret_access_key"
		}`,
	))

	creds := elastic.ResolveAWSCredentials(config, aws_config)
	credsValue, err := creds.Get()
	require.Nil(t, err, "Expect no error, %v", err)
	require.Equal(t, "static_access_key", credsValue.AccessKeyID, "Expect access key ID to match")
	require.Equal(t, "static_secret_access_key", credsValue.SecretAccessKey, "Expect secret access key to match")
}

func TestResolveAWSCredentialsEc2RoleProfile(t *testing.T) {
	server := initTestServer("2014-12-16T01:51:37Z", false)
	defer server.Close()

	aws_config := &aws.Config{
		Endpoint: aws.String(server.URL + "/latest"),
	}

	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["` + server.URL + `"],
			"aws":true,
			"aws_region":"us-east-1",
			"aws_profile":"test_aws_will_not_find"
		}`,
	))

	creds := elastic.ResolveAWSCredentials(config, aws_config)
	credsValue, err := creds.Get()
	require.Nil(t, err, "Expect no error, %v", err)
	require.Equal(t, "accessKey", credsValue.AccessKeyID, "Expect access key ID to match")
	require.Equal(t, "secret", credsValue.SecretAccessKey, "Expect secret access key to match")
}

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
	require.NoError(t, err)
	config.ProjectID = 633

	client, err := elastic.NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	require.NotNil(t, req)
	authRE := regexp.MustCompile(`\AAWS4-HMAC-SHA256 Credential=0/\d{8}/us-east-1/es/aws4_request, SignedHeaders=accept;content-type;date;host;x-amz-date, Signature=[a-f0-9]{64}\z`)
	require.Regexp(t, authRE, req.Header.Get("Authorization"))
	require.NotEqual(t, "", req.Header.Get("X-Amz-Date"))
}

func setupTestClient(t *testing.T) *elastic.Client {
	config := os.Getenv("ELASTIC_CONNECTION_INFO")
	if config == "" {
		t.Log("ELASTIC_CONNECTION_INFO not set")
		t.SkipNow()
	}

	os.Setenv("RAILS_ENV", fmt.Sprintf("test-elastic-%d", time.Now().Unix()))

	client, err := elastic.FromEnv()
	require.NoError(t, err)

	return client
}

func setupTestClientAndCreateIndex(t *testing.T) *elastic.Client {
	client := setupTestClient(t)

	t.Cleanup(func() {
		require.NoError(t, client.DeleteIndex())
	})

	require.NoError(t, client.CreateWorkingIndex())

	return client
}

func TestElasticClientIndexAndRetrieval(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	blobDoc := map[string]interface{}{}
	blobID := DocumentID{projectIDString, projectIDString + "_foo"}
	client.Index(&blobID, blobDoc)

	commitDoc := map[string]interface{}{}
	commitID := DocumentID{projectIDString, projectIDString + "_0000"}
	client.Index(&commitID, commitDoc)

	require.NoError(t, client.Flush())

	blob, err := client.Get(&blobID)
	require.NoError(t, err)
	require.Equal(t, true, blob.Found)

	commit, err := client.Get(&commitID)
	require.NoError(t, err)
	require.Equal(t, true, commit.Found)

	client.Remove(&blobID)
	require.NoError(t, client.Flush())

	_, err = client.Get(&blobID)
	require.Error(t, err)

	// indexing a doc with unexpected field will cause an ES strict_dynamic_mapping_exception
	// for our IndexMapping
	blobDocInvalid := map[string]interface{}{fmt.Sprintf("invalid-key-%d", time.Now().Unix()): ""}
	client.Index(&blobID, blobDocInvalid)
	require.Error(t, client.Flush())
}

func TestFlushErrorWithESActionRequestValidationException(t *testing.T) {
	client := setupTestClient(t)

	// set IndexName empty here to simulate ES action_request_validation_exception,
	// so that the `err` param passed to `afterFunc` is not nil
	client.IndexName = ""
	blobDoc := map[string]interface{}{}
	blobID := DocumentID{projectIDString, projectIDString + "_foo"}
	client.Index(&blobID, blobDoc)

	require.Error(t, client.Flush())
}

func TestElasticReadConfig(t *testing.T) {
	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["http://elasticsearch:9200"],
			"index_name": "foobar"
		}`,
	))
	require.NoError(t, err)

	require.Equal(t, "foobar", config.IndexName)
	require.Equal(t, []string{"http://elasticsearch:9200"}, config.URL)
	require.Equal(t, elastic.DefaultMaxBulkSize, config.MaxBulkSize)
	require.Equal(t, elastic.DefaultBulkWorkers, config.BulkWorkers)
}

func TestElasticReadConfigCustomBulkSettings(t *testing.T) {
	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"max_bulk_size_bytes": 1024,
			"max_bulk_concurrency": 6
		}`,
	))
	require.NoError(t, err)

	require.Equal(t, 1024, config.MaxBulkSize)
	require.Equal(t, 6, config.BulkWorkers)
}
