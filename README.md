# glay
[![go.dev reference](https://pkg.go.dev/badge/github.com/soypat/glay)](https://pkg.go.dev/github.com/soypat/glay)
[![Go Report Card](https://goreportcard.com/badge/github.com/soypat/glay)](https://goreportcard.com/report/github.com/soypat/glay)
[![codecov](https://codecov.io/gh/soypat/glay/branch/main/graph/badge.svg)](https://codecov.io/gh/soypat/glay)
[![Go](https://github.com/soypat/glay/actions/workflows/go.yml/badge.svg)](https://github.com/soypat/glay/actions/workflows/go.yml)
[![sourcegraph](https://sourcegraph.com/github.com/soypat/glay/-/badge.svg)](https://sourcegraph.com/github.com/soypat/glay?badge)


Go module template with instructions on how to make your code importable and setting up codecov CI.

How to install package with newer versions of Go (+1.16):
```sh
go mod download github.com/soypat/glay@latest
```
<!--
## Setting up codecov CI
This instructive will allow for tests to run on pull requests and pushes to your repository.

1. Create an account on [codecov.io](https://app.codecov.io/)

2. Setup repository on codecov and obtain the CODECOV_TOKEN token, which is a string of base64 characters.

3. Open up the github repository for this project and go to `Settings -> Secrets and variables -> Actions`. Once there create a New Repository Secret. Name it `CODECOV_TOKEN` and copy paste the token obtained in the previous step in the `secret` input box. Click "Add secret".

-->

