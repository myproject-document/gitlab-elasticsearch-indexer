package git

import (
	"io"
	"time"
)

type File struct {
	Path         string
	Blob         func() (io.ReadCloser, error)
	Oid          string
	SkipTooLarge bool
}

type Signature struct {
	Name  string
	Email string
	When  time.Time
}

type Commit struct {
	Author    Signature
	Committer Signature
	Message   string
	Hash      string
}

type Repository interface {
	EachFileChange(put PutFunc, del DelFunc) error
	EachCommit(f CommitFunc) error
	GetLimitFileSize() int64
}

type PutFunc func(file *File, fromCommit, toCommit string) error
type DelFunc func(path string) error
type CommitFunc func(commit *Commit) error
