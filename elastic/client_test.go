package elastic_test

import (
	"context"
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
	"github.com/aws/aws-sdk-go/aws/endpoints"
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

type testResolver struct {
	endpoint string
}

func (tr testResolver) EndpointFor(service, region string, opts ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
	return endpoints.ResolvedEndpoint{URL: tr.endpoint}, nil
}

func initTestServer(expireOn string, failAssume bool) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/meta-data/iam/security-credentials/":
			fmt.Fprintln(w, "RoleName")
		case "/latest/meta-data/iam/security-credentials/RoleName":
			if failAssume {
				fmt.Fprintf(w, "%s", credsFailRespTmpl)
			} else {
				fmt.Fprintf(w, credsRespTmpl, expireOn)
			}
		case "/gitlab-index-test/doc/667":
			time.Sleep(3 * time.Second)
			fmt.Fprintln(w, "{}")
		case "/latest/api/token":
			http.Error(w, "Not Found", http.StatusNotFound)
		default:
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))

	return server
}

func TestResolveAWSCredentialsStatic(t *testing.T) {
	require := require.New(t)

	awsConfig := &aws.Config{}
	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["http://localhost:9200"],
			"aws":true,
			"aws_access_key": "static_access_key",
			"aws_secret_access_key": "static_secret_access_key"
		}`,
	))
	require.NoError(err)

	creds := elastic.ResolveAWSCredentials(config, awsConfig)
	credsValue, err := creds.Get()
	require.NoError(err)
	require.Equal("static_access_key", credsValue.AccessKeyID, "Expect access key ID to match")
	require.Equal("static_secret_access_key", credsValue.SecretAccessKey, "Expect secret access key to match")
}

func TestResolveAWSEnvCredentials(t *testing.T) {
	require := require.New(t)

	awsConfig := &aws.Config{}
	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["http://localhost:9200"],
			"aws":true
		}`,
	))
	require.NoError(err)

	os.Setenv("AWS_ACCESS_KEY_ID", "id")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	os.Setenv("AWS_SESSION_TOKEN", "session-token")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_SESSION_TOKEN")
	}()

	creds := elastic.ResolveAWSCredentials(config, awsConfig)
	credsValue, err := creds.Get()
	require.NoError(err)
	require.Equal("id", credsValue.AccessKeyID, "Expect access key ID to match")
	require.Equal("secret", credsValue.SecretAccessKey, "Expect secret access key to match")
	require.Equal("session-token", credsValue.SessionToken, "Expect session token to match")
}

func TestResolveAWSCredentialsEc2RoleProfile(t *testing.T) {
	require := require.New(t)

	server := initTestServer("2014-12-16T01:51:37Z", false)
	defer server.Close()

	awsConfig := &aws.Config{
		EndpointResolver: testResolver{endpoint: server.URL + "/latest"},
	}

	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["` + server.URL + `"],
			"aws":true,
			"aws_region":"us-east-1",
			"aws_profile":"test_aws_will_not_find"
		}`,
	))
	require.NoError(err)

	// Bypass shared aws credential config file
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/notexist")
	defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")

	creds := elastic.ResolveAWSCredentials(config, awsConfig)
	credsValue, err := creds.Get()
	require.NoError(err)
	require.Equal("accessKey", credsValue.AccessKeyID, "Expect access key ID to match")
	require.Equal("secret", credsValue.SecretAccessKey, "Expect secret access key to match")
}

func TestResolveAWSCredentialsECSCredsProvider(t *testing.T) {
	require := require.New(t)

	server := initTestServer("2014-12-16T01:51:37Z", false)
	defer server.Close()

	awsConfig := &aws.Config{
		HTTPClient: &http.Client{},
	}

	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["` + server.URL + `"],
			"aws":true,
			"aws_region":"us-east-1",
			"aws_profile":"test_aws_will_not_find"
		}`,
	))
	require.NoError(err)

	// Bypass shared aws credential config file
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/notexist")
	os.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", server.URL+"/latest/meta-data/iam/security-credentials/RoleName")
	defer func() {
		defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
		defer os.Unsetenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")
	}()

	creds := elastic.ResolveAWSCredentials(config, awsConfig)
	credsValue, err := creds.Get()
	require.NoError(err)
	require.Equal("accessKey", credsValue.AccessKeyID, "Expect access key ID to match")
	require.Equal("secret", credsValue.SecretAccessKey, "Expect secret access key to match")
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

	client, err := elastic.NewClient(config, "the-correlation-id")
	require.NoError(t, err)
	// intiate a ClusterHealth API call to Elasticsearch since SetHealthcheck is set to false
	_, err = client.Client.ClusterHealth().Do(context.Background())
	require.NoError(t, err)
	defer client.Close()

	require.NotNil(t, req)
	authRE := regexp.MustCompile(`\AAWS4-HMAC-SHA256 Credential=0/\d{8}/us-east-1/es/aws4_request, SignedHeaders=accept;content-type;date;host;x-amz-date;x-opaque-id, Signature=[a-f0-9]{64}\z`)
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

	client, err := elastic.FromEnv(projectID, "test-correlation-id")
	require.NoError(t, err)

	require.Equal(t, projectID, client.ParentID())

	return client
}

func setupTestClientAndCreateIndex(t *testing.T) *elastic.Client {
	client := setupTestClient(t)
	require.NoError(t, client.CreateWorkingIndex())

	return client
}

func TestElasticClientIndexAndRetrieval(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	blobDoc := map[string]interface{}{}
	client.Index(projectIDString+"_foo", blobDoc)

	commitDoc := map[string]interface{}{}
	client.Index(projectIDString+"_0000", commitDoc)

	require.NoError(t, client.Flush())

	blob, err := client.GetBlob("foo")
	require.NoError(t, err)
	require.Equal(t, true, blob.Found)

	commit, err := client.GetCommit("0000")
	require.NoError(t, err)
	require.Equal(t, true, commit.Found)

	client.Remove(projectIDString + "_foo")
	require.NoError(t, client.Flush())

	_, err = client.GetBlob("foo")
	require.Error(t, err)

	// indexing a doc with unexpected field will cause an ES strict_dynamic_mapping_exception
	// for our IndexMapping
	blobDocInvalid := map[string]interface{}{fmt.Sprintf("invalid-key-%d", time.Now().Unix()): ""}
	client.Index(projectIDString+"_invalid", blobDocInvalid)
	require.Error(t, client.Flush())

	require.NoError(t, client.DeleteIndex())
}

func TestFlushErrorWithESActionRequestValidationException(t *testing.T) {
	client := setupTestClient(t)

	// set IndexName empty here to simulate ES action_request_validation_exception,
	// so that the `err` param passed to `afterFunc` is not nil
	client.IndexName = ""
	blobDoc := map[string]interface{}{}
	client.Index(projectIDString+"_foo", blobDoc)

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

func TestCorrelationIdForwardedAsXOpaqueId(t *testing.T) {
	var req *http.Request

	f := func(w http.ResponseWriter, r *http.Request) {
		req = r

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}

	srv := httptest.NewServer(http.HandlerFunc(f))
	defer srv.Close()

	config, err := elastic.ReadConfig(strings.NewReader(
		`{
			"url":["` + srv.URL + `"]
		}`,
	))
	require.NoError(t, err)
	config.ProjectID = projectID

	client, err := elastic.NewClient(config, "the-correlation-id")
	require.NoError(t, err)

	blobDoc := map[string]interface{}{}
	client.Index(projectIDString+"_foo", blobDoc)
	require.NoError(t, client.Flush())

	require.NotNil(t, req)
	require.Equal(t, "the-correlation-id", req.Header.Get("X-Opaque-Id"))
}

func TestClientTimeout(t *testing.T) {
	require := require.New(t)

	server := initTestServer("2014-12-16T01:51:37Z", false)
	defer server.Close()

	config := os.Getenv("ELASTIC_CONNECTION_INFO")
	os.Setenv(
		"ELASTIC_CONNECTION_INFO",
		`{
			"url": ["`+server.URL+`"],
			"index_name": "gitlab-index-test",
			"client_request_timeout": 1
		}`,
	)
	defer os.Setenv("ELASTIC_CONNECTION_INFO", config)

	client := setupTestClient(t)
	require.NotNil(client)

	_, err := client.Get(projectIDString)
	require.Error(
		err,
		"context deadline exceeded (Client.Timeout exceeded while awaiting headers)",
	)

	os.Setenv(
		"ELASTIC_CONNECTION_INFO",
		`{
			"url": ["`+server.URL+`"],
			"index_name": "gitlab-index-test"
		}`,
	)

	client = setupTestClient(t)
	require.NotNil(client)

	_, err = client.Get(projectIDString)
	require.NoError(err)
}

func TestHealthcheckIsDisabled(t *testing.T) {
	var req *http.Request

	f := func(w http.ResponseWriter, r *http.Request) {
		req = r
	}

	srv := httptest.NewServer(http.HandlerFunc(f))
	defer srv.Close()

	client := setupTestClient(t)
	require.NotNil(t, client)
	defer client.Close()

	require.Nil(t, req)
}
