module gitlab.com/gitlab-org/gitlab-elasticsearch-indexer

require (
	github.com/aws/aws-sdk-go v1.19.6
	github.com/deoxxa/aws_signing_client v0.0.0-20161109131055-c20ee106809e
	github.com/fortytw2/leaktest v1.3.0 // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/mailru/easyjson v0.0.0-20190403194419-1ea4449da983 // indirect
	github.com/olivere/elastic v6.2.24+incompatible
	github.com/sirupsen/logrus v1.4.1
	github.com/stretchr/testify v1.4.0
	gitlab.com/gitlab-org/gitaly v1.68.0
	gitlab.com/gitlab-org/labkit v0.0.0-20200507062444-0149780c759d
	gitlab.com/lupine/icu v1.0.0
	golang.org/x/net v0.0.0-20200114155413-6afb5195e5aa
	golang.org/x/tools v0.0.0-20200207001614-6fdc5776f4bb
	google.golang.org/grpc v1.24.0
)

go 1.12
