# regomark

Check Markdown documents with rego policies.

regomark reads a directory of Markdown documents into JSON that keeps the line of every block and link, resolves each link against the repository, and evaluates rego policies against that JSON with the OPA engine built in. The rules (which sections a document must have, how documents refer to each other, which links must resolve) are written in rego and tested with `opa test`; regomark itself knows nothing about any document format.

It is under development.

## Development

```bash
mise run build   # ./regomark
mise run test    # gofmt, go vet and go test with the race detector
mise run lint    # golangci-lint and go mod tidy
mise run vuln    # govulncheck
```

## License

[MIT](LICENSE)
