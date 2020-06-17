package main

import (
	"flag"
	"os"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/elastic"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/indexer"
)

var (
	versionFlag     = flag.Bool("version", false, "Print the version and exit")
	skipCommitsFlag = flag.Bool("skip-commits", false, "Skips indexing commits for the repo")
	blobTypeFlag    = flag.String("blob-type", "blob", "The type of blobs to index. Accepted values: 'blob', 'wiki_blob'")
	inputFileFlag   = flag.String("input-file", "", "TSV file containing the list of indexation tuples (project_id, repository_path, base_sha)")

	// Overriden in the makefile
	Version   = "dev"
	BuildTime = ""
)

func parseOperations() map[string]*indexer.IndexOperation {
	args := flag.Args()

	// Using a map ensure we can enforce that there is a single
	// operation per `RepositoryPath`, which is currently unsupported
	// by the indexation logic.
	blobType := *blobTypeFlag
	skipCommits := *skipCommitsFlag
	indexOperations := make(map[string]*indexer.IndexOperation)

	if *inputFileFlag == "" {
		if len(args) != 2 {
			log.Fatalf("Usage: %s [ --version | [--blob-type=(blob|wiki_blob)] [--skip-comits] (<project-id> <project-path> | --input-file <input-file>)]", os.Args[0])
		}

		// inline invocation, create a single `indexer.IndexOperation` from the args
		projectID, err := indexer.ParseProjectID(&args[0])
		if err != nil {
			log.Fatal(err)
		}

		repositoryPath := args[1]

		indexOperations[repositoryPath] = &indexer.IndexOperation{
			ProjectID:      projectID,
			RepositoryPath: repositoryPath,
			BaseSHA:        os.Getenv("FROM_SHA"),
			BlobType:       blobType,
			SkipCommits:    skipCommits,
		}
	} else {
		inputFile, err := os.Open(*inputFileFlag)
		if err != nil {
			log.Fatalf("Cannot open file at %s: %s.", *inputFileFlag, err)
		}

		reader := indexer.IndexOperationReader{
			Reader:      inputFile,
			BlobType:    blobType,
			SkipCommits: skipCommits,
		}

		if _, err := reader.ReadAllInto(indexOperations); err != nil {
			log.Fatalf("Cannot read index operations: %s", err)
		}
	}

	return indexOperations
}

func main() {
	configureLogger()
	flag.Parse()

	if *versionFlag {
		log.Printf("%s %s (built at: %s)", os.Args[0], Version, BuildTime)
		os.Exit(0)
	}

	operations := parseOperations()

	Index(operations)
}

func Index(operations map[string]*indexer.IndexOperation) elastic.Stats {
	esClient, err := elastic.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	// mbergeron: this is where we could leverage fanning-out
	for _, operation := range operations {
		indexedSHA, err := indexProject(operation, esClient)
		if err != nil {
			operation.ErrorCode = 1
			log.Errorf("Indexing error: %s", err)
		} else {
			operation.BaseSHA = indexedSHA
		}
	}

	// mbergeron: Wait after the last flush to make sure
	// the bulk request that contains the operations has been
	// sent successfully.
	//
	// We can't determine which operation were part of the
	// the failed bulk update, thus we need to assume they
	// all failed.
	err = esClient.Flush()
	for _, operation := range operations {
		if err != nil {
			operation.ErrorCode = 2
		}
		os.Stdout.WriteString(operation.Serialize() + "\n")
	}

	return esClient.Stats()
}

func indexProject(operation *indexer.IndexOperation, client *elastic.Client) (string, error) {
	repo, err := git.NewGitalyClientFromEnv(operation.RepositoryPath, operation.BaseSHA, "")
	if err != nil {
		return operation.BaseSHA, err
	}

	idx := &indexer.Indexer{
		ProjectID:  indexer.ProjectID(operation.ProjectID),
		Submitter:  client,
		Repository: repo,
	}

	log.Debugf("Indexing from %s to %s", repo.FromHash, repo.ToHash)
	log.Debugf("Index: %s, Project ID: %v, blob_type: %s, skip_commits?: %t", client.IndexName, operation.ProjectID, operation.BlobType, operation.SkipCommits)

	if err := idx.IndexBlobs(operation.BlobType); err != nil {
		return operation.BaseSHA, err
	}

	if !operation.SkipCommits && operation.BlobType == "blob" {
		if err := idx.IndexCommits(); err != nil {
			return operation.BaseSHA, err
		}
	}

	// This SHA will be used to update the project's `index_status` for incremental updates
	return repo.ToHash, nil
}

func configureLogger() {
	log.SetOutput(os.Stderr)
	_, debug := os.LookupEnv("DEBUG")

	if debug {
		log.SetLevel(log.DebugLevel)
	}
}
