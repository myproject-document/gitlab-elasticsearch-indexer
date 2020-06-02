package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

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

type InputTuple struct {
	indexer.ProjectID
	RepositoryPath string
	BaseSHA string
	BlobType string
	SkipCommits bool
	Success bool
}

func (i *InputTuple) Serialize() string {
	var successBit int64
	if i.Success {
		successBit = 1
	}
	
	return strings.Join(
		[]string{strconv.FormatInt(int64(i.ProjectID), 10), i.RepositoryPath, i.BaseSHA, strconv.FormatInt(successBit, 2)},
		"\t",
	)
}

func parseProjectID(s *string) (indexer.ProjectID, error) {
	projectID, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("Invalid ProjectID, got %s", *s) 
	}

	return indexer.ProjectID(projectID), nil
}

func parseOperations() *map[string]*InputTuple {
	args := flag.Args()

	// Using a map ensure we can enforce that there is a single
	// tuple by `RepositoryPath`, which is currently unsupported
	// by the indexation logic.
	inputs := make(map[string]*InputTuple)

	blobType := *blobTypeFlag
	skipCommits := *skipCommitsFlag

	if *inputFileFlag == "" {
 	  if len(args) != 2 {
	    log.Fatalf("Usage: %s [ --version | [--blob-type=(blob|wiki_blob)] [--skip-comits] (<project-id> <project-path> | --input-file)]", os.Args[0])
	  }

		// inline invocation, create a single `InputTuple` from the args
		projectID, err := parseProjectID(&args[0])
		if err != nil {
			log.Fatal(err)
		}
		
		repositoryPath := args[1]

		inputs[repositoryPath] = &InputTuple{
			ProjectID: projectID,
			RepositoryPath: repositoryPath,
			BaseSHA: os.Getenv("FROM_SHA"),
			BlobType: blobType,
			SkipCommits: skipCommits,
		}
	} else {
		inputFile, err := os.Open(*inputFileFlag)
		if (err != nil) {
			log.Fatalf("Cannot open file at %s: %s.", *inputFileFlag, err)
		}

		scanner := bufio.NewScanner(inputFile)

		for scanner.Scan() {
			parts := strings.Split(scanner.Text(), "\t")

			if l := len(parts); l != 3 {
				log.Fatalf("Missing arguments, expected 3 got %d", l) 
			}

			repositoryPath := parts[1]
			if _, exists := inputs[repositoryPath]; exists {
				log.Fatalf("Repositories can only be specified once, found '%s' multiple times", repositoryPath) 
			}

			projectID, err := parseProjectID(&parts[0])
			if err != nil {
				log.Fatal(err) 
			}

			input := &InputTuple{
				ProjectID: projectID,
				RepositoryPath: repositoryPath,
				BaseSHA: parts[2],
				BlobType: blobType,
				SkipCommits: skipCommits,
			}

			inputs[repositoryPath] = input
		}
	}

	return &inputs
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
			input.Success = false
			log.Errorf("Indexing error: %s", err)
		} else {
			input.BaseSHA = indexedSHA
			input.Success = true
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

func indexProject(input *InputTuple, client *elastic.Client) (string, error) {
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
