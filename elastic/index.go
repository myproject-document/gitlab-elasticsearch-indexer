package elastic

import (
	"context"
	"strconv"
	"strings"
)

// indexMapping is used as an example for testing purposes only and is 
// not intended to be kept synchronized with GitLab
const indexMapping = `
{
	"settings": {
		"analysis": {
			"filter": {
				"my_stemmer": {
					"name": "light_english",
					"type": "stemmer"
				},
				"code": {
					"type": "pattern_capture",
					"preserve_original": "true",
					"patterns": "(\\w+)"
				},
				"edgeNGram_filter": {
					"type": "edgeNGram",
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
				"code_search_analyzer": {
					"filter": [
						"lowercase",
						"asciifolding"
					],
					"type": "custom",
					"tokenizer": "whitespace"
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
						"code",
						"lowercase",
						"asciifolding",
						"edgeNGram_filter"
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
					"type": "nGram",
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
		"doc": {
			"dynamic": "strict",
			"_routing": {
				"required": true
			},
			"properties": __PROPERTIES__
		}
	}
}`

const indexProperties = `
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
				"search_analyzer": "code_search_analyzer",
				"type": "text"
			},
			"multi_purpose_content": {
				"type": "nGram",
				"index_options": "positions",
				"min_gram": 3,
				"max_gram": 3
			},
			"file_name": {
				"analyzer": "code_analyzer",
				"search_analyzer": "code_search_analyzer",
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

// createIndex creates an index matching that created by GitLab
func (c *Client) createIndex(mapping string) error {
	info, err := c.Client.NodesInfo().Do(context.Background())
	if err != nil {
		return err
	}

	createIndexService := c.Client.CreateIndex(c.IndexName).BodyString(mapping)

	for _, node := range info.Nodes {
		// Grab the first character of the version string and turn it into an int
		version, _ := strconv.Atoi(string(node.Version[0]))
		if version == 7 {
			// include_type_name defaults to false in ES7. This will ensure ES7
			// behaves like ES6 when creating mappings. See
			// https://www.elastic.co/blog/moving-from-types-to-typeless-apis-in-elasticsearch-7-0
			// for more information. We also can't set this for any versions before
			// 6.8 as this parameter was not supported. Since it defaults to true in
			// all 6.x it's safe to only set it for 7.x.
			createIndexService = createIndexService.IncludeTypeName(true)
		}

		// We only look at the first node and assume they're all the same version
		break
	}

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
func (c *Client) CreateWorkingIndex() error {
	mapping := strings.Replace(indexMapping, "__PROPERTIES__", indexProperties, -1)
		
	return c.createIndex(mapping)
}

// For testing
func (c *Client) CreateBrokenIndex() error {
	mapping := strings.Replace(indexMapping, "__PROPERTIES__", "{}", -1)

	return c.createIndex(mapping)
}

func (c *Client) DeleteIndex() error {
	deleteIndex, err := c.Client.DeleteIndex(c.IndexName).Do(context.Background())
	if err != nil {
		return err
	}

	if !deleteIndex.Acknowledged {
		return timeoutError
	}

	return nil
}
