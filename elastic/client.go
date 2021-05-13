package elastic

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/deoxxa/aws_signing_client"
	"github.com/olivere/elastic"
)

var (
	timeoutError        = fmt.Errorf("Timeout")
	envCorrelationIDKey = "CORRELATION_ID"
)

type Client struct {
	IndexName   string
	ProjectID   int64
	maxBulkSize int
	Client      *elastic.Client
	bulk        *elastic.BulkProcessor
	bulkFailed  bool
}

// FromEnv creates an Elasticsearch client from the `ELASTIC_CONNECTION_INFO`
// environment variable
func FromEnv(projectID int64, correlationID string) (*Client, error) {
	data := strings.NewReader(os.Getenv("ELASTIC_CONNECTION_INFO"))

	config, err := ReadConfig(data)
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse ELASTIC_CONNECTION_INFO: %s", err)
	}

	if config.IndexName == "" {
		railsEnv := os.Getenv("RAILS_ENV")
		indexName := "gitlab"
		if railsEnv != "" {
			indexName = indexName + "-" + railsEnv
		}
		config.IndexName = indexName
	}

	config.ProjectID = projectID

	return NewClient(config, correlationID)
}

func (c *Client) afterCallback(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
	if err != nil {
		c.bulkFailed = true

		if elastic.IsStatusCode(err, http.StatusRequestEntityTooLarge) {
			log.Printf("bulk request %d: error: %v, max bulk size setting (GitLab): %d bytes. Consider lowering maximum bulk request size or/and increasing http.max_content_length", executionId, err, c.maxBulkSize)
		} else {
			log.Printf("bulk request %d: error: %v", executionId, err)
		}
	}

	// bulk response can be nil in some cases, we must check first
	if response != nil && response.Errors {
		numFailed := len(response.Failed())
		if numFailed > 0 {
			c.bulkFailed = true
			total := numFailed + len(response.Succeeded())

			log.Printf("bulk request %d: failed to insert %d/%d documents ", executionId, numFailed, total)
		}
	}
}

func NewClient(config *Config, correlationID string) (*Client, error) {
	var opts []elastic.ClientOptionFunc

	httpClient := &http.Client{}
	if config.RequestTimeout != 0 {
		httpClient.Timeout = time.Duration(config.RequestTimeout) * time.Second
	}
	// AWS settings have to come first or they override custom URL, etc
	if config.AWS {
		awsConfig := &aws.Config{
			Region:     aws.String(config.Region),
			HTTPClient: &http.Client{},
		}
		credentials := ResolveAWSCredentials(config, awsConfig)
		signer := v4.NewSigner(credentials)
		awsClient, err := aws_signing_client.New(signer, httpClient, "es", config.Region)
		if err != nil {
			return nil, err
		}

		opts = append(opts, elastic.SetHttpClient(awsClient))
	} else {
		if config.RequestTimeout != 0 {
			opts = append(opts, elastic.SetHttpClient(httpClient))
		}
	}

	// Sniffer should look for HTTPS URLs if at-least-one initial URL is HTTPS
	for _, url := range config.URL {
		if strings.HasPrefix(url, "https:") {
			opts = append(opts, elastic.SetScheme("https"))
			break
		}
	}

	headers := http.Header{}
	headers.Add("X-Opaque-Id", correlationID)
	opts = append(opts, elastic.SetHeaders(headers))

	opts = append(opts, elastic.SetURL(config.URL...), elastic.SetSniff(false))

	opts = append(opts, elastic.SetHealthcheck(false))

	client, err := elastic.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	wrappedClient := &Client{
		IndexName:   config.IndexName,
		ProjectID:   config.ProjectID,
		maxBulkSize: config.MaxBulkSize,
		Client:      client,
	}

	bulk, err := client.BulkProcessor().
		Workers(config.BulkWorkers).
		BulkSize(config.MaxBulkSize).
		After(wrappedClient.afterCallback).
		Do(context.Background())

	if err != nil {
		return nil, err
	}

	wrappedClient.bulk = bulk

	return wrappedClient, nil
}

// ResolveAWSCredentials returns Credentials object
//
// Order of resolution
// 1.  Static Credentials - As configured in Indexer config
// 2.  Credentials from other providers
//     2a.  Credentials via env variables
//     2b.  Credentials via config files
//     2c.  ECS Role Credentials
//     2d.  EC2 Instance Role Credentials
func ResolveAWSCredentials(config *Config, awsConfig *aws.Config) *credentials.Credentials {
	providers := []credentials.Provider{
		&credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     config.AccessKey,
				SecretAccessKey: config.SecretKey,
			},
		},
	}
	providers = append(providers, defaults.CredProviders(awsConfig, defaults.Handlers())...)
	return credentials.NewChainCredentials(providers)
}

func (c *Client) ParentID() int64 {
	return c.ProjectID
}

func (c *Client) Flush() error {
	err := c.bulk.Flush()

	if err == nil && c.bulkFailed {
		err = fmt.Errorf("Failed to perform all operations")
	}

	return err
}

func (c *Client) Close() {
	c.Client.Stop()
}

func (c *Client) Index(id string, thing interface{}) {
	req := elastic.NewBulkIndexRequest().
		Index(c.IndexName).
		Type("doc").
		Routing(fmt.Sprintf("project_%v", c.ProjectID)).
		Id(id).
		Doc(thing)

	c.bulk.Add(req)
}

// We only really use this for tests
func (c *Client) Get(id string) (*elastic.GetResult, error) {
	return c.Client.Get().
		Index(c.IndexName).
		Type("doc").
		Routing(fmt.Sprintf("project_%v", c.ProjectID)).
		Id(id).
		Do(context.TODO())
}

func (c *Client) GetCommit(id string) (*elastic.GetResult, error) {
	return c.Get(fmt.Sprintf("%v_%v", c.ProjectID, id))
}

func (c *Client) GetBlob(path string) (*elastic.GetResult, error) {
	return c.Get(fmt.Sprintf("%v_%v", c.ProjectID, path))
}

func (c *Client) Remove(id string) {
	req := elastic.NewBulkDeleteRequest().
		Index(c.IndexName).
		Type("doc").
		Routing(fmt.Sprintf("project_%v", c.ProjectID)).
		Id(id)

	c.bulk.Add(req)
}
