package main

import (
	"bufio"
	"flag"
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
	// mbergeron: should we also put the `blobType` flag in the input-file tuple?
	inputFileFlag   = flag.String("input-file", "", "TSV file containing the list of indexation tuples (project_id, repository_path, base_sha)")

	// Overriden in the makefile
	Version   = "dev"
	BuildTime = ""
)

// mbergeron: use `json`
type InputTuple struct {
	ProjectID int64
	RepositoryPath string
	BaseSHA string
	BlobType string
	SkipCommits bool
}

func main() {
	flag.Parse()

	if *versionFlag {
		log.Printf("%s %s (built at: %s)", os.Args[0], Version, BuildTime)
		os.Exit(0)
	}

	configureLogger()
	args := flag.Args()

	// if len(args) != 2 {
	// 	log.Fatalf("Usage: %s [ --version | [--blob-type=(blob|wiki_blob)] [--skip-comits] (<project-id> <project-path> <base_sha> | --input-file)]", os.Args[0])
	// }

	blobType := *blobTypeFlag
	skipCommits := *skipCommitsFlag
	inputFilePath := flag.Lookup("input-file").Value.String()

	inputFile, err := os.Open(inputFilePath)
	if (err != nil) {
		log.Fatalf("Cannot open file at %s: %s.", inputFilePath, err)
	}

	scanner := bufio.NewScanner(inputFile)

	// Let's buffer up the inputs to close the file ASAP.
	var inputs []*InputTuple
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")

		if l := len(parts); l != 3 {
			log.Fatalf("Missing arguments, expected 3 got %d", l) 
		}

		projectID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			log.Fatalf("Invalid ProjectID, got %s", args[0]) 
		}

		input := &InputTuple{
			ProjectID: projectID,
			RepositoryPath: parts[1],
			BaseSHA: parts[2],
			BlobType: blobType,
			SkipCommits: skipCommits,
		}

		inputs = append(inputs, input)
	}

	esClient, err := elastic.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	// mbergeron: this is where we could leverage fanning-out
	for _, input := range inputs {
		var errors []error
		
		if err := indexProject(input, esClient); err != nil {
			errors = append(errors, err)
		}

		// mbergeron: we should output the list of `InputTuple` that had an error
		// in the same format

		for _, err := range errors {
			log.Error(err)
		}
	}

	if err := esClient.Flush(); err != nil {
		log.Fatalln("Flushing error: ", err)
	}
}

func indexProject(input *InputTuple, client *elastic.Client) error {
	repo, err := git.NewGitalyClientFromEnv(input.RepositoryPath, input.BaseSHA, "")
	if err != nil {
		// wrap the error
		log.Fatal(err)
	}

	idx := &indexer.Indexer{
		ProjectID: indexer.ProjectID(input.ProjectID),
		Submitter:  client,
		Repository: repo,
	}

	log.Debugf("Indexing from %s to %s", repo.FromHash, repo.ToHash)
	log.Debugf("Index: %s, Project ID: %v, blob_type: %s, skip_commits?: %t", client.IndexName, input.ProjectID, input.BlobType, input.SkipCommits)

	if err := idx.IndexBlobs(input.BlobType); err != nil {
		// wrap the error
		log.Fatalln("Indexing error: ", err)
	}

	if !input.SkipCommits && input.BlobType == "blob" {
		if err := idx.IndexCommits(); err != nil {
			// wrap the error
			log.Fatalln("Indexing error: ", err)
		}
	}

	return nil
}

func configureLogger() {
	log.SetOutput(os.Stdout)
	_, debug := os.LookupEnv("DEBUG")

	if debug {
		log.SetLevel(log.DebugLevel)
	}
}
