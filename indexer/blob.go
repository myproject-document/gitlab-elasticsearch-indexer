package indexer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"reflect"
	"strconv"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/linguist"
)

var (
	SkipTooLargeBlob = fmt.Errorf("Blob should be skipped: Too large")
	SkipBinaryBlob   = fmt.Errorf("Blob should be skipped: binary")
)

const (
	binarySearchLimit = 8 * 1024 // 8 KiB, Same as git
)

func isSkipBlobErr(err error) bool {
	switch err {
	case SkipTooLargeBlob:
		return true
	case SkipBinaryBlob:
		return true
	}

	return false
}

type Blob struct {
	Type      string
	ID        string
	OID       string
	RepoID    string
	CommitSHA string
	Content   string
	Path      string

	// Message copied from gitlab-elasticsearch-git:
	//
	// We're duplicating file_name parameter here because we need another
	// analyzer for it.
	//
	//Ideally this should be done with copy_to: 'blob.file_name' option
	//but it does not work in ES v2.3.*. We're doing it so to not make users
	//install newest versions
	//
	//https://github.com/elastic/elasticsearch-mapper-attachments/issues/124
	Filename string

	Language string

	FieldNameTable map[string]string
}

func GenerateBlobID(parentID int64, filename string) string {
	return fmt.Sprintf("%v_%s", parentID, filename)
}

func BuildBlob(file *git.File, parentID int64, commitSHA string, blobType string, mapping map[string]string) (*Blob, error) {
	if file.Size > git.LimitFileSize {
		return nil, SkipTooLargeBlob
	}

	reader, err := file.Blob()
	if err != nil {
		return nil, err
	}

	defer reader.Close()

	// FIXME(nick): This doesn't look cheap. Check the RAM & CPU pressure, esp.
	// for large blobs
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if DetectBinary(b) {
		return nil, SkipBinaryBlob
	}

	content := tryEncodeBytes(b)
	filename := tryEncodeString(file.Path)
	blob := &Blob{
		ID:        GenerateBlobID(parentID, filename),
		OID:       file.Oid,
		CommitSHA: commitSHA,
		Content:   content,
		Path:      filename,
		Filename:  path.Base(filename),
		Language:  DetectLanguage(filename, b),

		FieldNameTable: mapping,
	}

	switch blobType {
	case "blob":
		blob.Type = "blob"
		blob.RepoID = strconv.FormatInt(parentID, 10)
	case "wiki_blob":
		blob.Type = "wiki_blob"
		blob.RepoID = fmt.Sprintf("wiki_%d", parentID)
	}

	return blob, nil
}

// DetectLanguage returns a string describing the language of the file. This is
// programming language, rather than natural language.
//
// If no language is detected, "Text" is returned.
func DetectLanguage(filename string, data []byte) string {
	lang := linguist.DetectLanguage(filename, data)
	if lang != nil {
		return lang.Name
	}

	return "Text"
}

// DetectBinary checks whether the passed-in data contains a NUL byte. Only scan
// the start of large blobs. This is the same test performed by git to check
// text/binary
func DetectBinary(data []byte) bool {
	searchLimit := binarySearchLimit
	if len(data) < searchLimit {
		searchLimit = len(data)
	}

	return bytes.Contains(data[:searchLimit], []byte{0})
}

func (b *Blob) MarshalJSON() ([]byte, error) {
	out := map[string]interface{}{}

	s := reflect.ValueOf(b).Elem()
	typeOfT := s.Type()
	num := s.NumField()

	for i := 0; i < num; i++ {
		f := s.Field(i)

		key := typeOfT.Field(i).Name
		m := b.FieldNameTable
		if newKey, ok := m[key]; ok {
			out[newKey] = f.Interface()
		}
	}

	return json.Marshal(out)
}
