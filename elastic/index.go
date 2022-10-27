package elastic

import (
	"context"
	"strings"

	"github.com/olivere/elastic/v7"
)

// indexMapping is used as an example for testing purposes only and is
// not intended to be kept synchronized with GitLab
const defaultIndexMapping = `
{
	"settings": {
		"analysis": {
			"filter": {
				"my_stemmer": {
					"name": "light_english",
					"type": "stemmer"
				},
				"word_delimiter_graph_filter": {
					"type": "word_delimiter_graph",
					"preserve_original": "true"
				},
				"edge_ngram_filter": {
					"type": "edge_ngram",
					"min_gram": "2",
					"max_gram": "40"
				}
			},
			"analyzer": {
				"default": {
					"filter": [
						"lowercase",
						"my_stemmer"
					],
					"tokenizer": "standard"
				},
				"path_analyzer": {
					"filter": [
						"lowercase",
						"asciifolding"
					],
					"type": "custom",
					"tokenizer": "path_tokenizer"
				},
				"code_analyzer": {
					"filter": [
						"word_delimiter_graph_filter",
						"flatten_graph",
						"lowercase",
						"asciifolding",
						"edge_ngram_filter"
					],
					"type": "custom",
					"tokenizer": "whitespace"
				},
				"my_ngram_analyzer": {
					"filter": [
						"lowercase"
					],
					"tokenizer": "my_ngram_tokenizer"
				}
			},
			"tokenizer": {
				"my_ngram_tokenizer": {
					"token_chars": [
						"letter",
						"digit"
					],
					"min_gram": "2",
					"type": "ngram",
					"max_gram": "3"
				},
				"path_tokenizer": {
					"reverse": "true",
					"type": "path_hierarchy"
				}
			},
			"normalizer": {
				"sha_normalizer": {
				  "filter": [
					"lowercase"
				  ],
				  "type": "custom"
				}
			}
		}
	},
	"mappings": {
		"dynamic": "strict",
		"_routing": {
			"required": true
		},
		"properties": __PROPERTIES__
	}
}`

const defaultIndexProperties = `
{
	"archived": {
		"type": "boolean"
	},
	"assignee_id": {
		"type": "integer"
	},
	"author_id": {
		"type": "integer"
	},
	"blob": {
		"properties": {
			"commit_sha": {
				"normalizer": "sha_normalizer",
				"index_options": "docs",
				"type": "keyword"
			},
			"content": {
				"analyzer": "code_analyzer",
				"index_options": "positions",
				"type": "text"
			},
			"file_name": {
				"analyzer": "code_analyzer",
				"type": "text"
			},
			"id": {
				"normalizer": "sha_normalizer",
				"index_options": "docs",
				"type": "keyword"
			},
			"language": {
				"type": "keyword"
			},
			"oid": {
				"normalizer": "sha_normalizer",
				"index_options": "docs",
				"type": "keyword"
			},
			"path": {
				"analyzer": "path_analyzer",
				"type": "text"
			},
			"rid": {
				"type": "keyword"
			},
			"type": {
				"type": "keyword"
			}
		}
	},
	"commit": {
		"properties": {
			"author": {
				"properties": {
					"email": {
						"index_options": "docs",
						"type": "text"
					},
					"name": {
						"index_options": "docs",
						"type": "text"
					},
					"time": {
						"format": "basic_date_time_no_millis",
						"type": "date"
					}
				}
			},
			"committer": {
				"properties": {
					"email": {
						"index_options": "docs",
						"type": "text"
					},
					"name": {
						"index_options": "docs",
						"type": "text"
					},
					"time": {
						"format": "basic_date_time_no_millis",
						"type": "date"
					}
				}
			},
			"id": {
				"normalizer": "sha_normalizer",
				"index_options": "docs",
				"type": "keyword"
			},
			"message": {
				"index_options": "positions",
				"type": "text"
			},
			"rid": {
				"type": "keyword"
			},
			"sha": {
				"normalizer": "sha_normalizer",
				"index_options": "docs",
				"type": "keyword"
			},
			"type": {
				"type": "keyword"
			}
		}
	},
	"confidential": {
		"type": "boolean"
	},
	"content": {
		"index_options": "offsets",
		"type": "text"
	},
	"created_at": {
		"type": "date"
	},
	"description": {
		"index_options": "offsets",
		"type": "text"
	},
	"file_name": {
		"index_options": "offsets",
		"type": "text"
	},
	"id": {
		"type": "integer"
	},
	"iid": {
		"type": "integer"
	},
	"issue": {
		"properties": {
			"assignee_id": {
				"type": "integer"
			},
			"author_id": {
				"type": "integer"
			},
			"confidential": {
				"type": "boolean"
			}
		}
	},
	"issues_access_level": {
		"type": "integer"
	},
	"join_field": {
		"eager_global_ordinals": true,
		"relations": {
			"project": [
				"note",
				"blob",
				"issue",
				"milestone",
				"wiki_blob",
				"commit",
				"merge_request"
			]
		},
		"type": "join"
	},
	"last_activity_at": {
		"type": "date"
	},
	"last_pushed_at": {
		"type": "date"
	},
	"merge_requests_access_level": {
		"type": "integer"
	},
	"merge_status": {
		"type": "text"
	},
	"name": {
		"index_options": "offsets",
		"type": "text"
	},
	"name_with_namespace": {
		"analyzer": "my_ngram_analyzer",
		"index_options": "offsets",
		"type": "text"
	},
	"namespace_id": {
		"type": "integer"
	},
	"note": {
		"index_options": "offsets",
		"type": "text"
	},
	"noteable_id": {
		"type": "keyword"
	},
	"noteable_type": {
		"type": "keyword"
	},
	"path": {
		"index_options": "offsets",
		"type": "text"
	},
	"path_with_namespace": {
		"index_options": "offsets",
		"type": "text"
	},
	"project_id": {
		"type": "integer"
	},
	"repository_access_level": {
		"type": "integer"
	},
	"snippets_access_level": {
		"type": "integer"
	},
	"source_branch": {
		"index_options": "offsets",
		"type": "text"
	},
	"source_project_id": {
		"type": "integer"
	},
	"state": {
		"type": "text"
	},
	"target_branch": {
		"index_options": "offsets",
		"type": "text"
	},
	"target_project_id": {
		"type": "integer"
	},
	"title": {
		"index_options": "offsets",
		"type": "text"
	},
	"type": {
		"type": "keyword"
	},
	"updated_at": {
		"type": "date"
	},
	"visibility_level": {
		"type": "integer"
	},
	"wiki_access_level": {
		"type": "integer"
	}
}
`

const commitsIndexProperties = `
{
	"type": {
		"type": "keyword"
	},
	"visibility_level": {
		"type": "integer"
	},
	"repository_access_level": {
		"type": "integer"
	},
	"author": {
		"properties": {
			"email": {
				"index_options": "docs",
				"type": "text"
			},
			"name": {
				"index_options": "docs",
				"type": "text"
			},
			"time": {
				"format": "basic_date_time_no_millis",
				"type": "date"
			}
		}
	},
	"committer": {
		"properties": {
			"email": {
				"index_options": "docs",
				"type": "text"
			},
			"name": {
				"index_options": "docs",
				"type": "text"
			},
			"time": {
				"format": "basic_date_time_no_millis",
				"type": "date"
			}
		}
	},
	"id": {
		"normalizer": "sha_normalizer",
		"index_options": "docs",
		"type": "keyword"
	},
	"message": {
		"index_options": "positions",
		"type": "text"
	},
	"rid": {
		"type": "keyword"
	},
	"sha": {
		"normalizer": "sha_normalizer",
		"index_options": "docs",
		"type": "keyword"
	}
}
`

// createIndex creates an index matching that created by GitLab
func (c *Client) createIndex(indexName, mapping string) error {
	createIndexService := c.Client.CreateIndex(indexName).BodyString(mapping)

	createIndex, err := createIndexService.Do(context.Background())
	if err != nil {
		return err
	}

	if !createIndex.Acknowledged {
		return timeoutError
	}

	return nil
}

// CreateIndex creates an index matching that created by gitlab-rails.
func (c *Client) CreateDefaultWorkingIndex() error {
	mapping := strings.Replace(defaultIndexMapping, "__PROPERTIES__", defaultIndexProperties, -1)

	return c.createIndex(c.IndexNameDefault, mapping)
}

func (c *Client) CreateCommitsWorkingIndex() error {
	mapping := strings.Replace(defaultIndexMapping, "__PROPERTIES__", commitsIndexProperties, -1)

	return c.createIndex(c.IndexNameCommits, mapping)
}

// For testing
func (c *Client) CreateDefaultBrokenIndex() error {
	mapping := strings.Replace(defaultIndexMapping, "__PROPERTIES__", "{}", -1)

	return c.createIndex(c.IndexNameDefault, mapping)
}

func (c *Client) DeleteIndex(indexName string) error {
	deleteIndex, err := c.Client.DeleteIndex(indexName).Do(context.Background())
	if err != nil {
		return err
	}

	if !deleteIndex.Acknowledged {
		return timeoutError
	}

	return nil
}

type CreateAliasParams struct {
	Index        string
	Alias        string
	IsWriteIndex bool
}

func (c *Client) CreateAlias(params *CreateAliasParams) error {
	action := elastic.NewAliasAddAction(params.Alias).Index(params.Index).IsWriteIndex(params.IsWriteIndex)

	if err := action.Validate(); err != nil {
		return err
	}

	_, err := c.Client.Alias().Action(action).Do(context.TODO())

	return err
}

type RemoveAliasParams struct {
	Index string
	Alias string
}

func (c *Client) RemoveAlias(params *RemoveAliasParams) error {
	_, err := c.Client.Alias().
		Remove(params.Index, params.Alias).
		Do(context.TODO())

	return err
}
