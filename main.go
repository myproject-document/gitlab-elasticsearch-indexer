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

func parseOperations() *map[string]*indexer.IndexOperation {
	args := flag.Args()

	// Using a map ensure we can enforce that there is a single
	// operation per `RepositoryPath`, which is currently unsupported
	// by the indexation logic.
	blobType := *blobTypeFlag
	skipCommits := *skipCommitsFlag

	if *inputFileFlag == "" {
 	  if len(args) != 2 {
	    log.Fatalf("Usage: %s [ --version | [--blob-type=(blob|wiki_blob)] [--skip-comits] (<project-id> <project-path> | --input-file <input-file>)]", os.Args[0])
	  }

		// inline invocation, create a single `indexer.IndexOperation` from the args
		projectID, err := indexer.ParseProjectID(&args[0])
		if err != nil {
			log.Fatal(err)
		}
		
		inputs := make(map[string]*indexer.IndexOperation)
		repositoryPath := args[1]

		inputs[repositoryPath] = &indexer.IndexOperation{
			ProjectID: projectID,
			RepositoryPath: repositoryPath,
			BaseSHA: os.Getenv("FROM_SHA"),
			BlobType: blobType,
			SkipCommits: skipCommits,
		}

		return &inputs
	} else {
		inputFile, err := os.Open(*inputFileFlag)
		if (err != nil) {
			log.Fatalf("Cannot open file at %s: %s.", *inputFileFlag, err)
		}

		reader := indexer.IndexOperationReader{
			Reader: inputFile,
			BlobType: blobType,
			SkipCommits: skipCommits,
		}

		operations, err := reader.ReadOperations()
		return &operations
	}
}

func main() {
	configureLogger()
	flag.Parse()

	if *versionFlag {
		log.Printf("%s %s (built at: %s)", os.Args[0], Version, BuildTime)
		os.Exit(0)
	}

	inputs := parseOperations()

	esClient, err := elastic.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	// mbergeron: this is where we could leverage fanning-out
	for _, input := range *inputs {
		indexedSHA, err := indexProject(input, esClient)
		if err != nil {
			input.ErrorCode = 1
			log.Errorf("Indexing error: %s", err)
		} else {
			input.BaseSHA = indexedSHA
		}

		// mbergeron: The best approach here would be to only write this
		// whenever the bulk request that contains it has been flushed
		// successfully.
		//
		// This would enable the caller of this process to consume the
		// output as a stream of flush events.
		os.Stdout.WriteString(input.Serialize() + "\n")
	}

	if err := esClient.Flush(); err != nil {
		log.Fatalln("Flushing error: ", err)
	}
}

func indexProject(input *indexer.IndexOperation, client *elastic.Client) (string, error) {
	repo, err := git.NewGitalyClientFromEnv(input.RepositoryPath, input.BaseSHA, "")
	if err != nil {
		return input.BaseSHA, err
	}

	idx := &indexer.Indexer{
		ProjectID: indexer.ProjectID(input.ProjectID),
		Submitter:  client,
		Repository: repo,
	}

	log.Debugf("Indexing from %s to %s", repo.FromHash, repo.ToHash)
	log.Debugf("Index: %s, Project ID: %v, blob_type: %s, skip_commits?: %t", client.IndexName, input.ProjectID, input.BlobType, input.SkipCommits)

	if err := idx.IndexBlobs(input.BlobType); err != nil {
		return input.BaseSHA, err
	}

	if !input.SkipCommits && input.BlobType == "blob" {
		if err := idx.IndexCommits(); err != nil {
			return input.BaseSHA, err
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
