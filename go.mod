module gitlab.com/gitlab-org/gitlab-elasticsearch-indexer

go 1.17

require (
	github.com/aws/aws-sdk-go v1.42.23
	github.com/deoxxa/aws_signing_client v0.0.0-20161109131055-c20ee106809e
	github.com/go-enry/go-enry/v2 v2.7.1
	github.com/olivere/elastic/v7 v7.0.31
	github.com/stretchr/testify v1.7.0
	gitlab.com/gitlab-org/gitaly/v14 v14.4.2
	gitlab.com/gitlab-org/labkit v1.16.0
	gitlab.com/lupine/icu v1.0.0
	golang.org/x/net v0.0.0-20211209124913-491a49abca63
	golang.org/x/tools v0.1.5
	google.golang.org/grpc v1.48.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/client9/reopen v1.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-enry/go-oniguruma v1.2.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/hashicorp/yamux v0.0.0-20210316155119-a95892c5f864 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/oklog/ulid/v2 v2.0.2 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.12.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/sebest/xff v0.0.0-20210106013422-671bd2870b3a // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/genproto v0.0.0-20210813162853-db860fec028c // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)

exclude (
	// https://gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/-/issues/81
	github.com/gin-gonic/gin v1.4.0
	github.com/gin-gonic/gin v1.5.0
	github.com/gin-gonic/gin v1.6.0
	github.com/gin-gonic/gin v1.6.3
)
