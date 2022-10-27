package elastic

import (
	"context"
	"fmt"

	"net/http"
	"os"
	"strings"
	"time"

	logkit "gitlab.com/gitlab-org/labkit/log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/deoxxa/aws_signing_client"
	"github.com/olivere/elastic/v7"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

var (
	timeoutError = fmt.Errorf("Timeout")
)

type Client struct {
	IndexNameDefault string
	IndexNameCommits string
	ProjectID        int64
	Permissions      *indexer.ProjectPermissions
	maxBulkSize      int
	Client           *elastic.Client
	bulk             *elastic.BulkProcessor
	bulkFailed       bool
	SearchCuration   bool
}

// ConfigFromEnv creates a Config from the `ELASTIC_CONNECTION_INFO`
// environment variable
func ConfigFromEnv() (*Config, error) {
	data := strings.NewReader(os.Getenv("ELASTIC_CONNECTION_INFO"))

	config, err := ReadConfig(data)
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse ELASTIC_CONNECTION_INFO: %s", err)
	}

	if config.IndexNameDefault == "" {
		railsEnv := os.Getenv("RAILS_ENV")
		indexName := "gitlab"
		if railsEnv != "" {
			indexName = indexName + "-" + railsEnv
		}
		config.IndexNameDefault = indexName
	}

	return config, nil
}

func (c *Client) UseSeparateIndexForCommits() bool {
	return c.IndexNameCommits != "" && c.IndexNameCommits != c.IndexNameDefault
}

func (c *Client) afterCallback(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
	if err != nil {
		c.bulkFailed = true

		if elastic.IsStatusCode(err, http.StatusRequestEntityTooLarge) {
			logkit.WithFields(
				logkit.Fields{
					"bulkRequestId":      executionId,
					"maxBulkSizeSetting": c.maxBulkSize,
				},
			).WithError(err).Error("Consider lowering maximum bulk request size or/and increasing http.max_content_length")
		} else {
			logkit.WithFields(
				logkit.Fields{
					"bulkRequestId": executionId,
				},
			).WithError(err).Error("Bulk request failed")
		}
	}

	// bulk response can be nil in some cases, we must check first
	if response != nil && response.Errors {
		failedBulkResponseItems := response.Failed()
		numFailed := len(failedBulkResponseItems)
		if numFailed > 0 {
			c.bulkFailed = true
			total := numFailed + len(response.Succeeded())

			logkit.WithField("bulkRequestId", executionId).Errorf("Bulk request failed to insert %d/%d documents", numFailed, total)
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
		IndexNameDefault: config.IndexNameDefault,
		IndexNameCommits: config.IndexNameCommits,
		ProjectID:        config.ProjectID,
		Permissions:      config.Permissions,
		maxBulkSize:      config.MaxBulkSize,
		SearchCuration:   config.SearchCuration,
		Client:           client,
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

func (c *Client) ProjectPermissions() *indexer.ProjectPermissions {
	return c.Permissions
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

func (c *Client) indexNameFor(documentType string) string {
	if documentType == "commit" && c.IndexNameCommits != "" {
		return c.IndexNameCommits
	} else {
		return c.IndexNameDefault
	}
}

func (c *Client) Index(documentType, id string, thing interface{}) {
	indexName := c.indexNameFor(documentType)
	req := elastic.NewBulkIndexRequest().
		Index(indexName).
		Routing(fmt.Sprintf("project_%v", c.ProjectID)).
		Id(id).
		Doc(thing)

	if c.SearchCuration {
		c.DeleteFromRolledOverIndices(&RolloverParams{
			AliasName: indexName,
			DocType:   documentType,
			DocId:     id,
		})
	}

	c.bulk.Add(req)
}

type RolloverParams struct {
	AliasName string
	DocType   string
	DocId     string
}

func (c *Client) DeleteFromRolledOverIndices(params *RolloverParams) error {
	res, err := c.Client.Aliases().
		Index(params.AliasName).
		Pretty(true).
		Do(context.TODO())

	if err != nil {
		return err
	}

	// There are no rolled over indices yet
	if len(res.Indices) <= 1 {
		return nil
	}

	for indexName, indexDetails := range res.Indices {
		for _, aliasInfo := range indexDetails.Aliases {
			if aliasInfo.AliasName != params.AliasName || aliasInfo.IsWriteIndex {
				continue
			}

			logkit.WithFields(
				logkit.Fields{
					"search_curation": indexName,
					"doc_id":          params.DocId,
				},
			).Debugf("Deleting doc `%s` from rollover index %s", params.DocId, indexName)
			c.Remove(params.DocType, params.DocId)
		}
	}

	return nil
}

// We only really use this for tests
func (c *Client) Get(documentType, id string) (*elastic.GetResult, error) {
	return c.Client.Get().
		Index(c.indexNameFor(documentType)).
		Routing(fmt.Sprintf("project_%v", c.ProjectID)).
		Id(id).
		Do(context.TODO())
}

func (c *Client) GetCommit(id string) (*elastic.GetResult, error) {
	return c.Get("commit", fmt.Sprintf("%v_%v", c.ProjectID, id))
}

func (c *Client) GetBlob(path string) (*elastic.GetResult, error) {
	return c.Get("blob", fmt.Sprintf("%v_%v", c.ProjectID, path))
}

func (c *Client) Remove(documentType, id string) {
	req := elastic.NewBulkDeleteRequest().
		Index(c.indexNameFor(documentType)).
		Routing(fmt.Sprintf("project_%v", c.ProjectID)).
		Id(id)

	c.bulk.Add(req)
}
