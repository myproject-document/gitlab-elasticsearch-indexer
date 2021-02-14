# GitLab Elasticsearch Indexer

This project indexes Git repositories into Elasticsearch for GitLab. See the
[homepage](https://gitlab.com/gitlab-org/gitlab-elasticsearch-indexer) for more
information.

## Dependencies

This project relies on [ICU](http://site.icu-project.org/) for text encoding;
ensure the development packages for your platform are installed before running
`make`:

### Debian / Ubuntu

```
# apt install libicu-dev
```

### Mac OSX

```
$ brew install icu4c
$ export PKG_CONFIG_PATH="/usr/local/opt/icu4c/lib/pkgconfig:$PKG_CONFIG_PATH"
```

## Building & Installing

```
make
sudo make install
```

`gitlab-elasticsearch-indexer` will be installed to `/usr/local/bin`

You can change the installation path with the `PREFIX` env variable. Please remember to pass the `-E` flag to sudo if you do so.

Example:
```
PREFIX=/usr sudo -E make install
```

## Run tests

Test suite expects Gitaly and Elasticsearch to be run, on the following ports:

  - Gitaly: 8075
  - ElasticSearch v7.9.2: 9201

### Quick tests

```bash
# you only have to run this once, as it starts the services
make test-infra

# source the default connections
source .env.test

# run the test suite
make test

# or run a specific test
go test -v gitlab.com/gitlab-org/gitlab-elasticsearch-indexer -run TestIndexingGitlabTest

```

If you want to re-create the infra, you can run `make test-infra` again.

### Custom tests

If you want to test a particular setup, for instance:

  - You want to run on a local Gitaly instance, as the one from the GDK
  - You want to use a specific ElasticSearch cluster, as the one from the GDK

Then you'll have to manually set the proper tests connections.

First, start the services that you need (`gitlab`, `elasticsearch`), with using `docker-compose up <service> -d`


```bash
# to start Gitaly
docker-compose up gitaly -d

# to start ElasticSearch
docker-compose up elasticsearch -d
```

Before running tests, set configuration variables

```bash
# these are the defaults, in `.env.test`

export GITALY_CONNECTION_INFO='{"address": "tcp://localhost:8075", "storage": "default"}'
export ELASTIC_CONNECTION_INFO='{"url":["http://localhost:9201"], "index_name":"gitlab-test"}'
```

**Note**: If using a socket, please pass your URI in the form `unix://FULL_PATH_WITH_LEADING_SLASH`

Example:
```bash
# source the default connections
source .env.test

# override the Gitaly connection
export GITALY_CONNECTION_INFO='{"address": "unix:///gitlab/gdk/gitaly.socket", "storage": "default"}'

# run the test suite
make test

# or a specific test
go test -v gitlab.com/gitlab-org/gitlab-elasticsearch-indexer -run TestIndexingGitlabTest
```

### Testing in gdk

You can test changes to the indexer in your GDK by modifying [GITLAB_ELASTICSEARCH_INDEXER_VERSION](https://gitlab.com/gitlab-org/gitlab/-/blob/main/GITLAB_ELASTICSEARCH_INDEXER_VERSION) to point to the branch name containing the changes.


## Default branch

GitLab Elasticsearch Indexer is transitioning its default branch from `master` to `main`. For now,
both branches are valid. All changes go to the `main` branch and are synced manually
to `master` by the maintainers. We plan to remove the `master` branch as soon as
possible. The current status is being tracked in [issue 71](https://gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/-/issues/71).

## Contributing

Please see the [contribution guidelines](CONTRIBUTING.md)
