package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"strconv"
	"time"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
	"gitlab.com/gitlab-org/labkit/correlation"
	logkit "gitlab.com/gitlab-org/labkit/log"
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
	closer, err := configureLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing logkit %v", err)
		os.Exit(1)
	}
	defer closer.Close()

	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(os.Stdout, "%s %s (built at: %s)", os.Args[0], Version, BuildTime)
		os.Exit(0)
	}

	args := flag.Args()

	if len(args) != 2 {
		error := errors.New("WrongArguments")
		logkit.WithError(error).Fatalf("Usage: %s [ --version | [--blob-type=(blob|wiki_blob)] [--skip-commits] [--project-path=project-path] [--timeout=timeout] [--visbility-level=visbility-level] [--repository-access-level=repository-access-level] project-id repository-path ]", os.Args[0])
	}

	projectID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		logkit.WithError(err).Fatalf("Error parsing projectID %s", args[0])
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
		logkit.WithFields(
			logkit.Fields{
				"repoPath":      repoPath,
				"fromSHA":       fromSHA,
				"toSHA":         toSHA,
				"correlationID": correlationID,
				"projectID":     args[0],
				"projectPath":   projectPath,
			},
		).WithError(err).Fatal("Error creating gitaly client")
	}

	config, err := loadConfig(projectID)
	if err != nil {
		logkit.WithError(err).Fatalf("Error loading config for projectID %d", projectID)
	}

	esClient, err := elastic.NewClient(config, correlationID)
	if err != nil {
		logkit.WithError(err).Fatal("Error creating elastic client")
	}

	if timeoutOption != "" {
		timeout, err := time.ParseDuration(timeoutOption)
		if err != nil {
			logkit.WithError(err).Fatalf("Error parsing timeout %s", timeoutOption)
		} else {
			logkit.WithField("timeout", timeout).Info("Setting timeout")

			time.AfterFunc(timeout, func() {
				error := errors.New("TimedOut")
				logkit.WithError(error).Fatalf("The process has timed out after %s", timeout)
			})
		}
	}

	idx := indexer.NewIndexer(repo, esClient)

	logkit.WithFields(
		logkit.Fields{
			"IndexNameDefault": esClient.IndexNameDefault,
			"IndexNameCommits": esClient.IndexNameCommits,
			"projectID":        esClient.ParentID(),
			"blobType":         blobType,
			"skipCommits":      skipCommits,
			"Permissions":      config.Permissions,
		},
	).Debugf("Indexing from %s to %s", repo.FromHash, repo.ToHash)

	if err := idx.IndexBlobs(blobType); err != nil {
		logkit.WithError(err).Fatalln("Indexing error")
	}

	if !skipCommits && blobType == "blob" {
		if err := idx.IndexCommits(); err != nil {
			logkit.WithError(err).Fatalln("Indexing error")
		}
	}

	if err := idx.Flush(); err != nil {
		logkit.WithError(err).Fatalln("Flushing error")
	}
}

func configureLogger() (io.Closer, error) {
	_, debug := os.LookupEnv("DEBUG")

	level := "info"
	if debug {
		level = "debug"
	}

	logPath, logPathExists := os.LookupEnv("INDEXER_LOG")
	outputName := "stdout"
	if logPathExists {
		outputName = logPath
	}

	return logkit.Initialize(
		logkit.WithLogLevel(level),
		logkit.WithFormatter("json"),
		logkit.WithOutputName(outputName),
	)
}

func loadConfig(projectID int64) (*elastic.Config, error) {
	config, err := elastic.ConfigFromEnv()
	config.Permissions = generateProjectPermissions()
	config.ProjectID = projectID

	return config, err
}

func generateCorrelationID() string {
	var err error

	cid := os.Getenv(envCorrelationIDKey)
	if cid == "" {
		if cid, err = correlation.RandomID(); err != nil {
			// Should never happen since correlation.RandomID() should not fail,
			// but if it does we return empty string, which is fine.
			logkit.WithError(err).Error("Unable to generate random correlation ID")
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
