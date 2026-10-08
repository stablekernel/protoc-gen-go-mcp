Contributing
------------
When contributing to this repository, please first discuss the change you wish to make via issue, email, or any other method with the owners of this repository before making a change.

Please note we have a [code of conduct](CODE_OF_CONDUCT.md), please follow it in all your interactions with the project.

### Issues
1. Please open an issue for any bugs or feature requests. If you are unsure if your issue is a bug or a feature request, please open an issue and we will help you determine the best course of action.
2. Issues will be marked ready for work when they are ready to be worked on. If you would like to work on an issue that is not marked ready for work, please comment on the issue but do know a different path might be taken subsequently.
3. If you are working on an issue, please assign yourself to it. If you are not sure if you want to work on an issue, please comment on the issue and we will help you determine the best course of action.

### Pull Requests
1. Please branch off the main branch using `git checkout -b <my-initials/#issue-number-brief-description>`.
2. Make your changes, and ensure that your code passes all tests. We prefer that you use small commits for each logical change with a descriptive commit message.
3. Maintainers will be notified of your pull request and will review it. If you have any questions, please feel free to ask.
4. Make sure you leave a good description of your changes in the pull request. This will help us understand your changes and make it easier for us to review them. Also please refernce the issue that this fixes in the description, eg `Fixes #1234`.
5. Please follow our code of conduct in all your interactions with the project. Good discourse is always welcome and we view a healthy discourse on both PRs and issues as a sign of a healthy project.

### Running the tests
Run these before opening a pull request, and paste the commands and their
results in the PR description (see [`AGENTS.md`](AGENTS.md)):

```sh
gofmt -l .                # must print nothing
go vet ./...
go build ./...
go test ./...
```

### Updating the golden file
`cmd/protoc-gen-go-mcp/golden_test.go` compares the plugin's generated code
byte-for-byte against the committed
[`examples/gen/example/v1/example_mcp.pb.go`](examples/gen/example/v1/example_mcp.pb.go).
If you change the generator (`cmd/protoc-gen-go-mcp/**`) in a way that
deliberately changes its output, update the golden file and review the diff —
an unexpected change is a bug until explained:

```sh
go test ./cmd/protoc-gen-go-mcp -run Golden -update
```

### Regenerating the examples
If you changed the generator or `examples/protos/example.proto`, regenerate
the committed example code and commit the result in the same PR:

```sh
make generate   # needs protoc 29.3, protoc-gen-go v1.36.6, protoc-gen-go-grpc v1.5.1 on PATH
git status      # the regenerated files belong in the same PR
```

A CI job also fails the build if the committed examples are out of date with
what `make generate` would produce.
