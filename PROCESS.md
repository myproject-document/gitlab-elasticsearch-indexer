# GitLab Elasticsearch Indexer development process

## Maintainers

GitLab Elasticsearch Indexer has the following maintainers:

- Dylan Griffith `@DylanGriffith`
- Dmitry Gruzd `@dgruzd`
- Terri Chu `@terrichu`

This list is defined at https://about.gitlab.com/team/

### How to become a maintainer

GitLab Elasticsearch Indexer follows a maintainership process based on the [smaller project
template](https://about.gitlab.com/handbook/engineering/workflow/code-review/#maintainership-process-for-smaller-projects). 

Anyone may nominate themselves as a trainee by opening a tracking issue using the [`gitlab-elasticsearch-indexer` trainee maintainer template](https://gitlab.com/gitlab-com/www-gitlab-com/-/issues/new?issuable_template=trainee-gitlab-elasticsearch-indexer-maintainer&issue[title]=gitlab-elasticsearch-indexer%20Trainee%20Maintainer%3A%20%5BFull%20Name%5D). It's normally a good idea to check with at least one maintainer or your manager before creating the issue, but it's not required.

## Merging and reviewing contributions

Contributions must be reviewed by at least one maintainer. The final merge must
be performed by a maintainer.

## Releases

New versions can be released by one of the maintainers. The release process is:

-   pick a release branch. For x.y.0, use `main`. For all other
    versions (x.y.1, x.y.2 etc.) , use `x-y-stable`. Also see [below](#versioning)
-   create a merge request to update CHANGELOG and VERSION on the
    release branch
-   merge the merge request
-   run `make tag` or `make signed_tag` on the release branch. This will
    make a tag matching the VERSION file.
-   push the tag to gitlab.com

## Versioning

GitLab Elasticsearch Indexer uses a variation of SemVer. We don't use
"normal" SemVer because we have to be able to integrate into GitLab stable
branches.

A version has the format MAJOR.MINOR.PATCH.

- Major and minor releases are tagged on the `main` branch
- If the change is backwards compatible, increment the MINOR counter
- If the change breaks compatibility, increment MAJOR and set MINOR to `0`
- Patch release tags must be made on stable branches
- Only make a patch release when targeting a GitLab stable branch

This means that tags that end in `.0` (e.g. `8.5.0`) must always be on
the main branch, and tags that end in anything other than `.0` (e.g.
`8.5.2`) must always be on a stable branch.

> The reason we do this is that SemVer suggests something like a
> refactoring constitutes a "patch release", while the GitLab stable
> branch quality standards do not allow for back-porting refactorings
> into a stable branch.
