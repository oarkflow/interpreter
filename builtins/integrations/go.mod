module github.com/oarkflow/interpreter/builtins/integrations

go 1.25.7

require (
	github.com/jlaffaye/ftp v0.2.0
	github.com/oarkflow/interpreter v0.0.0
	github.com/pkg/sftp v1.13.10
	golang.org/x/crypto v0.51.0
)

require (
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/kr/fs v0.1.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
)

replace github.com/oarkflow/interpreter => ../..
