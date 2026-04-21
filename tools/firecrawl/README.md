# sonalmod


## Project Setup

Please have the following tools installed: 
* [direnv](https://github.com/direnv/direnv) 
* [gobrew](https://github.com/kevincobain2000/gobrew#install-or-update)

Install/Update dependencies: 
```sh
# Install
go mod download
go get -u tool
go install tool

# Update:
go get -u ./... && go mod tidy
```

## Development

### Lint and Tests

Run all lint and tests:
```bash
make lint
make test
```