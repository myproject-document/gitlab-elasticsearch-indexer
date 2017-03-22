package git

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"gopkg.in/libgit2/git2go.v25"
)

var (
	ZeroHash                 = &git.Oid{}
	submoduleFilemode uint16 = 0160000
)

type git2GoRepository struct {
	Repository *git.Repository

	FromHash *git.Oid
	ToHash   *git.Oid

	FromCommit *git.Commit
	ToCommit   *git.Commit
}

func NewGit2GoRepository(projectPath string, fromSHA string, toSHA string) (*git2GoRepository, error) {
	out := &git2GoRepository{}

	repo, err := git.OpenRepository(projectPath)
	if err != nil {
		return nil, err
	}
	out.Repository = repo

	if fromSHA == "" {
		fromSHA = ZeroHash.String()
	}

	out.FromHash, err = git.NewOid(fromSHA)
	if err != nil {
		return nil, err
	}

	if !out.FromHash.IsZero() {
		commit, err := out.Repository.LookupCommit(out.FromHash)
		if err != nil {
			return nil, fmt.Errorf("Bad from SHA (%s): %s", out.FromHash, err)
		}

		out.FromCommit = commit
	}

	if toSHA == "" {
		ref, err := out.Repository.Head()
		if err != nil {
			return nil, err
		}

		out.ToHash = ref.Target()
	} else {
		oid, err := git.NewOid(toSHA)
		if err != nil {
			return nil, err
		}

		out.ToHash = oid
	}

	commit, err := out.Repository.LookupCommit(out.ToHash)
	if err != nil {
		return nil, fmt.Errorf("Bad to SHA (%s): %s", out.ToHash, err)
	}

	out.ToCommit = commit

	return out, nil
}

func (r *git2GoRepository) diff() (*git.Diff, error) {
	var fromTree, toTree *git.Tree

	if r.FromCommit != nil {
		tree, err := r.FromCommit.Tree()
		if err != nil {
			return nil, err
		}

		fromTree = tree
	}

	toTree, err := r.ToCommit.Tree()
	if err != nil {
		return nil, err
	}

	defOpts, err := git.DefaultDiffOptions()
	if err != nil {
		return nil, err
	}

	opts := &defOpts
	opts.IgnoreSubmodules = git.SubmoduleIgnoreAll
	opts.Flags = defOpts.Flags | git.DiffIgnoreSubmodules

	return r.Repository.DiffTreeToTree(fromTree, toTree, opts)
}

func git2GoBuildSignature(sig *git.Signature) Signature {
	return Signature{
		Name:  sig.Name,
		Email: sig.Email,
		When:  sig.When,
	}
}

func (r *git2GoRepository) callout(file git.DiffFile, next FileFunc) error {
	blob, err := r.Repository.LookupBlob(file.Oid)
	if err != nil {
		return err
	}

	f := &File{
		Path: file.Path,
		Oid:  file.Oid.String(),
		Blob: func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(blob.Contents())), nil
		},
		Size: blob.Size(),
	}

	return next(f, r.FromHash.String(), r.ToHash.String())
}

func (r *git2GoRepository) EachFileChange(ins, mod, del FileFunc) error {
	diff, err := r.diff()
	if err != nil {
		return err
	}

	numDeltas, err := diff.NumDeltas()
	if err != nil {
		return err
	}

	for i := 0; i < numDeltas; i++ {
		delta, err := diff.GetDelta(i)
		if err != nil {
			return err
		}

		if delta.OldFile.Mode == submoduleFilemode {
			continue
		}

		if delta.NewFile.Mode == submoduleFilemode {
			continue
		}

		switch delta.Status {
		case git.DeltaAdded, git.DeltaCopied:
			err = r.callout(delta.NewFile, ins)
		case git.DeltaModified:
			err = r.callout(delta.NewFile, mod)
		case git.DeltaDeleted:
			err = r.callout(delta.OldFile, del)
		case git.DeltaRenamed:
			err = r.callout(delta.OldFile, del)
			if err == nil {
				err = r.callout(delta.NewFile, ins)
			}
		default:
			err = fmt.Errorf("Unrecognised change calculating diff: %+v", delta)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (r *git2GoRepository) EachCommit(f CommitFunc) error {
	rev, err := r.Repository.Walk()
	if err != nil {
		return err
	}
	//	defer rev.Free()

	if err := rev.Push(r.ToHash); err != nil {
		return err
	}

	var outErr error
	iterator := func(c *git.Commit) bool {
		commit := &Commit{
			Message:   c.Message(),
			Hash:      c.Id().String(),
			Author:    git2GoBuildSignature(c.Author()),
			Committer: git2GoBuildSignature(c.Committer()),
		}

		// abort walking due to reaching the termination point
		if c.Id().Equal(r.FromHash) {
			return false
		}

		outErr = f(commit)

		// Abort walking due to an error
		if outErr != nil {
			return false
		}

		return true
	}

	if err := rev.Iterate(iterator); err != nil {
		return fmt.Errorf("WalkCommitHistory: %s", err)
	}

	return outErr
}
