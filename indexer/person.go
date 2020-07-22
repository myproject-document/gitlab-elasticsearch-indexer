package indexer

import (
	"time"

	"gitlab.com/gitlab-org/gitlab-elasticsearch-indexer/git"
)

const (
	elasticTimeFormat = "20060102T150405-0700"
)

type Person struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Time  string `json:"time"` // %Y%m%dT%H%M%S%z
}

func GenerateDate(t time.Time) string {
	return t.Format(elasticTimeFormat)
}

func BuildPerson(p git.Signature, encoder *Encoder) *Person {
	return &Person{
		Name:  encoder.tryEncodeString(p.Name),
		Email: encoder.tryEncodeString(p.Email),
		Time:  GenerateDate(p.When),
	}
}
