# kubectl fd

## Commit instructions

- Run `pre-commit` before committing
- Commit messages must follow [Conventional Commits v1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)

## Testing instructions

- Run unit tests with `go test -v ./...`

- For integration test use `kind` or another k8s in docker solution, populate needed objects and test.

## Action compatibility

- Before implementing a new action or enabling an existing action for another resource type, write a failing regression test for the intended handler and resource.
- Keep an explicit action × handler compatibility matrix in tests. It must cover every action accepted by the CLI for Pod, Node, and universal resource handlers, including expected rejections.
- Flag parsing alone is insufficient: tests must execute the action through the selected handler and assert the API operation or resulting object state.
- If an action is not supported for a resource type, reject it during command validation with an actionable error; do not let it reach a handler as an "unsupported action" error.

## PR instructions

- Title format must be the same as commit message format
- Always run `golangci-lint run` and `go test -v ./...` before committing to PR.
