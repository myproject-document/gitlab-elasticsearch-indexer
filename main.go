package main

import (
	"flag"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
	"gitlab.com/gitlab-org/labkit/correlation"
)

var (
	versionFlag               = flag.Bool("version", false, "Print the version and exit")
	skipCommitsFlag           = flag.Bool("skip-commits", false, "Skips indexing commits for the repo")
	blobTypeFlag              = flag.String("blob-type", "blob", "The type of blobs to index. Accepted values: 'blob', 'wiki_blob'")
	visibilityLevelFlag       = flag.Int("visibility-level", -1, "Project visbility_access_level. Accepted values: 0, 10, 20")
	repositoryAccessLevelFlag = flag.Int("repository-access-level", -1, "Project repository_access_level. Accepted values: 0, 10, 20")
	projectPathFlag           = flag.String("project-path", "", "Project path")
	timeoutOptionFlag         = flag.String("timeout", "", "The timeout of the process.  Empty string means no timeout. Accepted formats: '1s', '5m', '24h'")

	// Overriden in the makefile
	Version   = "dev"
	BuildTime = ""

	envCorrelationIDKey = "CORRELATION_ID"
	Permissions         *indexer.ProjectPermissions
)

func main() {
	flag.Parse()

	if *versionFlag {
		log.Printf("%s %s (built at: %s)", os.Args[0], Version, BuildTime)
		os.Exit(0)
	}

	configureLogger()
	args := flag.Args()

	if len(args) != 2 {
		log.Fatalf("Usage: %s [ --version | [--blob-type=(blob|wiki_blob)] [--skip-commits] [--project-path=<project-path>] [--timeout=<timeout>] [--visbility-level=<visbility-level>] [--repository-access-level=<repository-access-level>] <project-id> <repo-path> ]", os.Args[0])
	}

	projectID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	repoPath := args[1]

	fromSHA := os.Getenv("FROM_SHA")
	toSHA := os.Getenv("TO_SHA")
	blobType := *blobTypeFlag
	skipCommits := *skipCommitsFlag
	projectPath := *projectPathFlag
	timeoutOption := *timeoutOptionFlag
	correlationID := generateCorrelationID()

	repo, err := git.NewGitalyClientFromEnv(repoPath, fromSHA, toSHA, correlationID, args[0], projectPath)
	if err != nil {
		log.Fatal(err)
	}

	config := loadConfig(projectID)

	esClient, err := elastic.NewClient(config, correlationID)
	if err != nil {
		log.Fatal(err)
	}

	if timeoutOption != "" {
		timeout, err := time.ParseDuration(timeoutOption)
		if err != nil {
			log.Fatal(err)
		} else {
			log.Infof("Setting timeout to %s", timeout)

			time.AfterFunc(timeout, func() {
				log.Fatalf("The process has timed out after %s", timeout)
			})
		}
	}

	idx := indexer.NewIndexer(repo, esClient)

	log.Debugf("Indexing from %s to %s", repo.FromHash, repo.ToHash)
	log.Debugf("Default Index Name: %s, Commits Index Name: %s, Project ID: %v, blob_type: %s, skip_commits?: %t, Permissions: %v", esClient.IndexNameDefault, esClient.IndexNameCommits, esClient.ParentID(), blobType, skipCommits, Permissions)

	if err := idx.IndexBlobs(blobType); err != nil {
		log.Fatalln("Indexing error: ", err)
	}

	if !skipCommits && blobType == "blob" {
		if err := idx.IndexCommits(); err != nil {
			log.Fatalln("Indexing error: ", err)
		}
	}

	if err := idx.Flush(); err != nil {
		log.Fatalln("Flushing error: ", err)
	}
}

func configureLogger() {
	log.SetOutput(os.Stdout)
	_, debug := os.LookupEnv("DEBUG")

	if debug {
		log.SetLevel(log.DebugLevel)
	}
}

func loadConfig(projectID int64) *elastic.Config {
	config, err := elastic.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	config.Permissions = generateProjectPermissions()
	config.ProjectID = projectID

	return config
}

func generateCorrelationID() string {
	var err error

	cid := os.Getenv(envCorrelationIDKey)
	if cid == "" {
		if cid, err = correlation.RandomID(); err != nil {
			// Should never happen since correlation.RandomID() should not fail,
			// but if it does we return empty string, which is fine.
			log.Error("Unable to generate random correlation ID: ", err)
		}
	}

	return cid
}

func generateProjectPermissions() *indexer.ProjectPermissions {
	visibilityLevel := *visibilityLevelFlag
	repositoryAccessLevel := *repositoryAccessLevelFlag

	if visibilityLevel == -1 || repositoryAccessLevel == -1 {
		return nil
	}

	permissions := new(indexer.ProjectPermissions)
	permissions.VisibilityLevel = int8(visibilityLevel)
	permissions.RepositoryAccessLevel = int8(repositoryAccessLevel)

	return permissions
}
